package main

import (
  "bufio"
  "encoding/json"
  "fmt"
  "io"
  "log"
  "os"
  "os/exec"
  "os/signal"
  "regexp"
  "strings"
  "syscall"
)

var pagesProgressRegex = regexp.MustCompile("^c(\\d+)/(\\d+)$")
var bytesProgressRegex = regexp.MustCompile("^b(\\d+)/(\\d+)$")
var fractionProgressRegex = regexp.MustCompile("^0(?:.\\d+)?$")

type Task struct {
  Blob struct {
    NBytes int64 `json:nBytes`
  } `json:blob`
}

func downloadBlob(inputJson string, mimeBoundary string) {
  var task Task
  if err := json.NewDecoder(strings.NewReader(inputJson)).Decode(&task); err != nil {
    log.Fatalf("Could not parse input JSON: %s", err)
  }

  blobFile, err := os.OpenFile("input.blob", os.O_CREATE|os.O_WRONLY, 0644)
  if err != nil {
    log.Fatalf("Could not open input.blob for writing: %s", err)
  }
  defer blobFile.Close()

  nBytes, err := io.Copy(blobFile, os.Stdin)
  if err != nil {
    log.Fatalf("Could not copy from stdin to input.blob: %s", err)
  }

  if nBytes != task.Blob.NBytes {
    message := fmt.Sprintf("Input had wrong length: read %d bytes, but input JSON specified %d bytes", nBytes, task.Blob.NBytes)
    printErrorAndExit(message, mimeBoundary)
  }
}

func printFragment(name string, contents string, mimeBoundary string) {
  if _, err := os.Stdout.Write([]byte("--" + mimeBoundary + "\r\nContent-Type: multipart/form-data; name=" + name + "\r\n\r\n" + contents + "\r\n")); err != nil {
    log.Fatalf("Error writing: %s", err)
  }
}

func printLineAsFragment(line string, mimeBoundary string) {
  if g := pagesProgressRegex.FindStringSubmatch(line); g != nil {
    printFragment("progress", "{\"children\":{\"nProcessed\":" + g[1] + ",\"nTotal\":" + g[2] + "}}", mimeBoundary)
  } else if g := bytesProgressRegex.FindStringSubmatch(line); g != nil {
    printFragment("progress", "{\"bytes\":{\"nProcessed\":" + g[1] + ",\"nTotal\":" + g[2] + "}}", mimeBoundary)
  } else if fractionProgressRegex.MatchString(line) {
    printFragment("progress", line, mimeBoundary)
  } else {
    printErrorAndExit(line, mimeBoundary)
  }
}

func printErrorAndExit(message string, mimeBoundary string) {
  if _, err := os.Stdout.Write([]byte("--" + mimeBoundary + "\r\nContent-Type: multipart/form-data; name=error\r\n\r\n" + message + "\r\n--" + mimeBoundary + "--")); err != nil {
    log.Fatalf("Error writing: %s", err)
  }
  os.Exit(0)
}

func printDoneAndExit(mimeBoundary string) {
  if _, err := os.Stdout.Write([]byte("--" + mimeBoundary + "\r\nContent-Type: multipart/form-data; name=done\r\n\r\n\r\n--" + mimeBoundary + "--")); err != nil {
    log.Fatalf("Error writing: %s", err)
  }
  os.Exit(0)
}

func printFileAsFragment(path string, mimeBoundary string) {
  file, err := os.Open(path)
  if err != nil {
    printErrorAndExit("do-convert-single-file did not output " + path, mimeBoundary)
  }

  if _, err := os.Stdout.Write([]byte("--" + mimeBoundary + "\r\nContent-Type: multipart/form-data; name=" + path + "\r\n\r\n")); err != nil {
    log.Fatalf("Error writing: %s", err)
  }
  if _, err := io.Copy(os.Stdout, file); err != nil {
    log.Fatalf("Error copying %s: %s", path, err)
  }
  if _, err := os.Stdout.Write([]byte("\r\n")); err != nil {
    log.Fatalf("Error writing: %s", err)
  }
}

func printFileAsFragmentIfExists(path string, mimeBoundary string) {
  if _, err := os.Stat(path); err == nil {
    printFileAsFragment(path, mimeBoundary)
  }
}

func runConvert(mimeBoundary string) {
  path := "/app/do-convert-single-file"
  cmd := exec.Cmd { Path: path }

  stderr, err := cmd.StderrPipe()
  if err != nil {
    log.Fatalf("Could not open stderr for read: %s", err)
  }

  stdout, err := cmd.StdoutPipe()
  if err != nil {
    log.Fatalf("Could not open stdout for read: %s", err)
  }

  if err := cmd.Start(); err != nil {
    if os.IsNotExist(err) {
      printErrorAndExit(path + " does not exist or is not executable", mimeBoundary)
    } else {
      log.Fatalf("Could not start %s: %s", path, err)
    }
  }

  // Pipe stderr to self
  go func() {
    if _, err := io.Copy(os.Stderr, stderr); err != nil {
      log.Printf("io.Copy(os.Stderr, stderr) failed: %s", err)
    }
  }()

  // Convert stdout while piping
  go func() {
    scanner := bufio.NewScanner(stdout)
    for scanner.Scan() {
      printLineAsFragment(scanner.Text(), mimeBoundary)
    }
  }()

  interrupt := make(chan os.Signal, 1)
  signal.Notify(interrupt, os.Interrupt)

  go func() {
    <-interrupt
    cmd.Process.Kill()
  }()

  if err := cmd.Wait(); err != nil {
    if exiterr, ok := err.(*exec.ExitError); ok {
      // Buggy program exited with nonzero.
      if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
        message := fmt.Sprintf("do-convert-single-file exited with status code %d", status.ExitStatus())
        printErrorAndExit(message, mimeBoundary)
      } else {
        log.Fatalf("Could not determine exit code")
      }
    } else {
      log.Fatalf("convert-single-file failed: %s", err)
    }
  }

  close(interrupt)

  printFileAsFragment("0.json", mimeBoundary)
  printFileAsFragment("0.blob", mimeBoundary)
  printFileAsFragmentIfExists("0.jpg", mimeBoundary)
  printFileAsFragmentIfExists("0.png", mimeBoundary)
  printFileAsFragmentIfExists("0.txt", mimeBoundary)
  printDoneAndExit(mimeBoundary)
}

func doConvert(mimeBoundary string, inputJson string) {
  downloadBlob(inputJson, mimeBoundary)
  runConvert(mimeBoundary)
}

func main() {
  mimeBoundary := os.Args[1]
  inputJson := os.Args[2]

  doConvert(mimeBoundary, inputJson)
}
