package main

import (
  "bufio"
  "encoding/json"
  "fmt"
  "io"
  "log"
  "os"
  "os/exec"
  "net/http"
  "regexp"
  "syscall"
)

var pagesProgressRegex = regexp.MustCompile("^c(\\d+)/(\\d+)$")
var bytesProgressRegex = regexp.MustCompile("^b(\\d+)/(\\d+)$")
var fractionProgressRegex = regexp.MustCompile("^0(?:.\\d+)?$")

type Task struct {
  Url string `json:url`
}

func downloadBlob() error {
  jsonFile, err := os.Open("input.json")
  if err != nil {
    return err
  }
  defer jsonFile.Close()

  var task Task
  if err := json.NewDecoder(jsonFile).Decode(&task); err != nil {
    return err
  }

  resp, err := http.Get(task.Url)
  if err != nil {
    // TODO handle 404 (race in which task was canceled); crash on other errors
    return err
  }
  defer resp.Body.Close()

  blobFile, err := os.OpenFile("input.blob", os.O_CREATE|os.O_WRONLY, 0644)
  if err != nil {
    return err
  }
  if _, err := io.Copy(blobFile, resp.Body); err != nil {
    return err
  }

  return nil
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
    log.Fatalf("Could not start %s: %s", path, err)
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

  printFileAsFragment("0.json", mimeBoundary)
  printFileAsFragment("0.blob", mimeBoundary)
  printDoneAndExit(mimeBoundary)
}

func doConvert(mimeBoundary string) {
  if err := downloadBlob(); err != nil {
    panic(err)
  }

  runConvert(mimeBoundary)
}

func main() {
  mimeBoundary := os.Args[1]

  doConvert(mimeBoundary)
}
