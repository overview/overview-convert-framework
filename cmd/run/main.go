package main

import (
  "bytes"
  "encoding/json"
  "io"
  "io/ioutil"
  "log"
  "math/rand"
  "net"
  "net/http"
  "net/url"
  "os"
  "os/exec"
  "strings"
  "syscall"
  "time"
)

const retryTimeout = 10 * time.Second

type Task struct {
  Url string `json:url`
  Blob struct {
    Url string `json:url`
  }
}

func generateMimeBoundary() []byte {
  // https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-golang
  const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
  const nBytes = 50 // bytes
  ret := make([]byte, nBytes)
  for i := range ret {
    ret[i] = letters[rand.Intn(len(letters))]
  }
  return ret
}

func runConvert(task Task, jsonBytes []byte) {
  mimeBoundary := string(generateMimeBoundary())

  path := "/app/convert"
  args := make([]string, 3)
  args[0] = path
  args[1] = mimeBoundary
  args[2] = string(jsonBytes)
  cmd := exec.Cmd {
    Path: path,
    Args: args,
  }

  stdin, err := cmd.StdinPipe()
  if err != nil {
    log.Fatalf("Could not open stdin from /app/convert: %s", err)
  }

  stderr, err := cmd.StderrPipe()
  if err != nil {
    log.Fatalf("Could not open stderr from /app/convert: %s", err)
  }

  stdout, err := cmd.StdoutPipe()
  if err != nil {
    log.Fatalf("Could not open stdout from /app/convert: %s", err)
  }

  if err := cmd.Start(); err != nil {
    log.Fatalf("Could not invoke /app/convert: %s", err)
  }

  // Pipe stderr to self
  go func() {
    if _, err := io.Copy(os.Stderr, stderr); err != nil {
      log.Printf("io.Copy(os.Stderr, stderr) failed: %s", err)
    }
  }()

  // Pipe job to stdin
  go func() {
    resp, err := http.Get(task.Blob.Url)
    if err != nil {
      if resp.StatusCode != 200 {
        // TODO handle non-fatal errors: 404 -> return
        log.Fatalf("Overview gave us a task but no blob: %s", task.Blob.Url)
      }
      log.Printf("GET %s: %s", task.Blob.Url, err)
      stdin.Close()
      return
    }
    defer resp.Body.Close()
    if _, err := io.Copy(stdin, resp.Body); err != nil {
      log.Printf("Failed to copy blob to stdin: %s")
    }
    stdin.Close()
  }()

  // Pipe stdout to url
  resp, err := http.Post(task.Url, "multipart/form-data; boundary=\"" + mimeBoundary + "\"", stdout)
  if err != nil {
    log.Printf("POST piping /app/convert output failed: %s", err)
  }
  // TODO assert HTTP 202 Accepted
  // TODO handle server going away
  resp.Body.Close()

  if err := cmd.Wait(); err != nil {
    log.Fatalf("/app/convert did not return with status code 0. That means it has a bug.")
  }
}

func tick(pollUrl string, retryTimeout time.Duration) {
  resp, err := http.Get(pollUrl)
  if err != nil {
    if uerr, ok := err.(*url.Error); ok {
      if operr, ok := uerr.Err.(*net.OpError); ok {
        if scerr, ok := operr.Err.(*os.SyscallError); ok {
          if scerr.Err == syscall.ECONNREFUSED {
            log.Printf("Connection refused; will retry in %fs", retryTimeout)
            time.Sleep(retryTimeout)
            return
          }

          log.Fatalf("Unhandled os.SyscallError: %v", scerr.Err)
        } else if dnserr, ok := operr.Err.(*net.DNSError); ok && dnserr.IsTemporary {
          log.Printf("%s; will try in %fs", dnserr.Error(), retryTimeout)
          time.Sleep(retryTimeout)
          return
        } else if strings.HasSuffix(operr.Err.Error(), ": no such host") {
          log.Printf("DNS lookup failed for %s: no such host; will try in %fs", pollUrl, retryTimeout)
          time.Sleep(retryTimeout)
          return
        }

        log.Fatalf("Unhandled net.OpError: %v", operr.Err)
      }
    }

    log.Fatalf("Unhandled HTTP error: %#v", err)
  }
  defer resp.Body.Close()
  if resp.StatusCode == 204 {
    log.Printf("Overview has no tasks for us; will retry in %fs", retryTimeout)
    time.Sleep(retryTimeout)
    return
  } else if resp.StatusCode != 200 {
    log.Fatalf("Overview responded with status %s", resp.Status)
  }

  jsonBytes, err := ioutil.ReadAll(resp.Body)
  if err != nil {
    log.Fatalf("Could not receive JSON task from Overview: %s", err)
  }

  var task Task
  jsonDecoder := json.NewDecoder(bytes.NewReader(jsonBytes))
  if err := jsonDecoder.Decode(&task); err != nil {
    log.Fatalf("Could not parse JSON task from Overview: %s", err)
  }

  runConvert(task, jsonBytes)
}

func main() {
  pollUrl := os.Getenv("POLL_URL")
  if pollUrl == "" {
    panic("You must set POLL_URL before calling this program")
  }

  rand.Seed(time.Now().UnixNano())

  if len(os.Args) > 1 && os.Args[1] == "just-one-tick" {
    tick(pollUrl, 0 * time.Second)
  } else {
    for {
      tick(pollUrl, retryTimeout)
    }
  }
}
