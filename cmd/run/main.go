package main

import (
  "bytes"
  "encoding/json"
  "io"
  "io/ioutil"
  "log"
  "math/rand"
  "net/http"
  "os"
  "os/exec"
  "time"
)

type Task struct {
  Url string `json:url`
  Blob struct {
    Url string `json:url`
  }
}

func writeInputJson(tmpdir string, jsonBytes []byte) error {
  f, err := os.OpenFile("input.json", os.O_CREATE|os.O_WRONLY, 0644)
  if err != nil {
    return err
  }
  if _, err := f.Write(jsonBytes); err != nil {
    return err
  }
  if err := f.Close(); err != nil {
    return err
  }
  return nil
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

  args := make([]string, 2)
  args[0] = mimeBoundary
  args[1] = string(jsonBytes)
  cmd := exec.Cmd {
    Path: "/app/convert",
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
      // TODO handle non-fatal errors: 404 -> return
      log.Printf("GET %s: %s", task.Blob.Url, err)
      stdin.Close()
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
  resp.Body.Close()

  if err := cmd.Wait(); err != nil {
    log.Fatalf("/app/convert did not return with status code 0. That means it has a bug.")
  }
}

func tick(url string) {
  resp, err := http.Get(url)
  if err != nil {
    // TODO handle non-fatal errors: 404 -> retry; non-response -> wait and retry
    log.Fatalf("Unhandled HTTP error: %s", err)
  }
  defer resp.Body.Close()

  jsonBytes, err := ioutil.ReadAll(resp.Body)
  var task Task
  jsonDecoder := json.NewDecoder(bytes.NewReader(jsonBytes))
  if err := jsonDecoder.Decode(&task); err != nil {
    log.Fatalf("Could not parse JSON task from Overview: %s", err)
  }

  runConvert(task, jsonBytes)
}

func main() {
  url := os.Getenv("POLL_URL")
  if url == "" {
    panic("You must set POLL_URL before calling this program")
  }

  rand.Seed(time.Now().UnixNano())

  if os.Args[1] == "just-one-tick" {
    tick(url)
  } else {
    for {
      tick(url)
    }
  }
}
