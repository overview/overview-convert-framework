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

func prepareTempDir(tempDir string) {
  if err := os.RemoveAll(tempDir); err != nil {
    log.Fatalf("Failed to empty %s: %s", tempDir, err)
  }

  if err := os.MkdirAll(tempDir, 0755); err != nil {
    log.Fatalf("Failed to create %s: %s", tempDir, err)
  }
}

func writeInputBlob(inputJson string, tempDir string, mimeBoundary string) {
  var task Task
  if err := json.NewDecoder(strings.NewReader(inputJson)).Decode(&task); err != nil {
    log.Fatalf("Could not parse input JSON: %s", err)
  }

  blobFile, err := os.OpenFile(tempDir + "/input.blob", os.O_CREATE|os.O_WRONLY, 0644)
  if err != nil {
    log.Fatalf("Could not open %s/input.blob for writing: %s", tempDir, err)
  }
  defer blobFile.Close()

  nBytes, err := io.Copy(blobFile, os.Stdin)
  if err != nil {
    log.Fatalf("Could not copy from stdin to %s/input.blob: %s", tempDir, err)
  }

  if nBytes != task.Blob.NBytes {
    message := fmt.Sprintf("Input had wrong length: read %d bytes, but input JSON specified %d bytes", nBytes, task.Blob.NBytes)
    printErrorAndExit(message, mimeBoundary)
  }
}

func printFragment(name string, contents string, mimeBoundary string) {
  if _, err := os.Stdout.Write([]byte("--" + mimeBoundary + "\r\nContent-Disposition: form-data; name=" + name + "\r\n\r\n" + contents + "\r\n")); err != nil {
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
  if _, err := os.Stdout.Write([]byte("--" + mimeBoundary + "\r\nContent-Disposition: form-data; name=error\r\n\r\n" + message + "\r\n--" + mimeBoundary + "--")); err != nil {
    log.Fatalf("Error writing: %s", err)
  }
  os.Stdout.Close()
  os.Exit(0)
}

func printDoneAndExit(mimeBoundary string) {
  if _, err := os.Stdout.Write([]byte("--" + mimeBoundary + "\r\nContent-Disposition: form-data; name=done\r\n\r\n\r\n--" + mimeBoundary + "--")); err != nil {
    log.Fatalf("Error writing: %s", err)
  }
  os.Stdout.Close()
  os.Exit(0)
}

func printFileAsFragment(tempDir string, path string, mimeBoundary string) {
  file, err := os.Open(tempDir + "/" + path)
  if err != nil {
    printErrorAndExit("do-convert-single-file did not output " + path, mimeBoundary)
  }

  if _, err := os.Stdout.Write([]byte("--" + mimeBoundary + "\r\nContent-Disposition: form-data; name=" + path + "\r\n\r\n")); err != nil {
    log.Fatalf("Error writing: %s", err)
  }
  if _, err := io.Copy(os.Stdout, file); err != nil {
    log.Fatalf("Error copying %s: %s", path, err)
  }
  if _, err := os.Stdout.Write([]byte("\r\n")); err != nil {
    log.Fatalf("Error writing: %s", err)
  }
}

func printFileAsFragmentIfExists(tempDir string, path string, mimeBoundary string) {
  if _, err := os.Stat(tempDir + "/" + path); err == nil {
    printFileAsFragment(tempDir, path, mimeBoundary)
  }
}

func runConvert(mimeBoundary string, inputJson string, tempDir string) {
  path := "/app/do-convert-single-file"
  args := make([]string, 2)
  args[0] = path
  args[1] = inputJson
  cmd := exec.Cmd {
    Path: path,
    Args: args,
    Dir: tempDir,
    Stderr: os.Stderr,
  }

  stdout, err := cmd.StdoutPipe()
  if err != nil {
    log.Fatalf("Could not open stdout for read: %s", err)
  }

  interrupt := make(chan os.Signal, 1)
  signal.Notify(interrupt, os.Interrupt)

  if err := cmd.Start(); err != nil {
    if os.IsNotExist(err) {
      printErrorAndExit(path + " does not exist or is not executable", mimeBoundary)
    } else {
      log.Fatalf("Could not start %s: %s", path, err)
    }
  }

  go func() {
    <-interrupt
    cmd.Process.Signal(os.Interrupt)
    cmd.Wait()
    os.Exit(0)
  }()

  scanner := bufio.NewScanner(stdout)
  for scanner.Scan() {
    printLineAsFragment(scanner.Text(), mimeBoundary)
  }

  if err = cmd.Wait(); err != nil {
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

  printFileAsFragment(tempDir, "0.json", mimeBoundary)
  printFileAsFragment(tempDir, "0.blob", mimeBoundary)
  printFileAsFragmentIfExists(tempDir, "0-thumbnail.jpg", mimeBoundary)
  printFileAsFragmentIfExists(tempDir, "0-thumbnail.png", mimeBoundary)
  printFileAsFragmentIfExists(tempDir, "0.txt", mimeBoundary)
  printDoneAndExit(mimeBoundary)
}

func doConvert(mimeBoundary string, inputJson string, tempDir string) {
  prepareTempDir(tempDir)
  writeInputBlob(inputJson, tempDir, mimeBoundary)
  runConvert(mimeBoundary, inputJson, tempDir)
}

func main() {
  log.SetFlags(0)

  mimeBoundary := os.Args[1]
  inputJson := os.Args[2]
  tempDir := os.TempDir() + "/overview-convert-single-file"

  doConvert(mimeBoundary, inputJson, tempDir)
}
