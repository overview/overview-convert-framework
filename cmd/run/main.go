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
}

func prepareTempdir(tmpdir string) error {
  err := os.RemoveAll(tmpdir)
  if err != nil {
    return err
  }

  err = os.MkdirAll(tmpdir, 0755)
  return err
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

func runConvert(url string, tempdir string) error {
  mimeBoundary := string(generateMimeBoundary())

  args := make([]string, 1)
  args[0] = mimeBoundary
  cmd := exec.Cmd {
    Path: "/app/convert",
    Args: args,
    Dir: tempdir,
  }

  stderr, err := cmd.StderrPipe()
  if err != nil {
    return err
  }

  stdout, err := cmd.StdoutPipe()
  if err != nil {
    return err
  }

  if err := cmd.Start(); err != nil {
    return err
  }

  // Pipe stderr to self
  go func() {
    if _, err := io.Copy(os.Stderr, stderr); err != nil {
      log.Printf("io.Copy(os.Stderr, stderr) failed: %s", err)
    }
  }()

  // Pipe stdout to url
  resp, err := http.Post(url, "multipart/form-data; boundary=\"" + mimeBoundary + "\"", stdout)
  if err != nil {
    log.Printf("POST piping /app/convert output failed: %s", err)
  }
  // TODO assert HTTP 202 Accepted
  resp.Body.Close()

  if err := cmd.Wait(); err != nil {
    // /app/convert MUST exit with status code 0. Non-zero means framework is
    // broken.
    return err
  }

  return nil
}

func tick(url string, tempdir string) error {
  resp, err := http.Get(url)
  if err != nil {
    // TODO handle non-fatal errors: 404 -> retry; non-response -> wait and retry
    return err
  }
  defer resp.Body.Close()

  err = prepareTempdir(tempdir)
  if err != nil {
    return err
  }

  jsonBytes, err := ioutil.ReadAll(resp.Body)
  var task Task
  jsonDecoder := json.NewDecoder(bytes.NewReader(jsonBytes))
  if err := jsonDecoder.Decode(&task); err != nil {
    // Server gave us gibberish. That's a fatal error.
    return err
  }

  if err := writeInputJson(tempdir, jsonBytes); err != nil {
    return err
  }

  return runConvert(url, tempdir)
}

func main() {
  url := os.Getenv("POLL_URL")
  if url == "" {
    panic("You must set POLL_URL before calling this program")
  }

  dir := os.TempDir() + "/overview-convert-run"
  rand.Seed(time.Now().UnixNano())

  for {
    if err := tick(url, dir); err != nil {
      panic(err)
    }
  }
}
