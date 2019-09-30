package main

import (
  "bufio"
  "bytes"
  "fmt"
  "io"
  "log"
  "os"
  "os/exec"
  "os/signal"
  "regexp"
  "syscall"
)

func prepareTempDir(tempDir string) {
  if err := os.RemoveAll(tempDir); err != nil {
    log.Fatalf("Failed to empty %s: %s", tempDir, err)
  }

  if err := os.MkdirAll(tempDir, 0755); err != nil {
    log.Fatalf("Failed to create %s: %s", tempDir, err)
  }
}

func outputOrCrash(b []byte) {
  if _, err := os.Stdout.Write(b); err != nil {
    log.Fatalf("Error writing to stdout: %s", err)
  }
}

func printFragment(name string, contents string, mimeBoundary string) {
  // This may be called before any other message. We'll still write the initial
  // \r\n: it makes for an empty "preamble" in RFC2046, which is valid. And it
  // makes this file's logic easier.
  outputOrCrash([]byte("\r\n--" + mimeBoundary + "\r\nContent-Disposition: form-data; name=" + name + "\r\n\r\n" + contents))
}

func printCloseDelimiter(mimeBoundary string) {
  outputOrCrash([]byte("\r\n--" + mimeBoundary + "--"))
}

func printErrorAndExit(message string, mimeBoundary string) {
  printFragment("error", message, mimeBoundary)
  printCloseDelimiter(mimeBoundary)
  os.Exit(0)
}

func min(x, y int) int {
  if x < y {
    return x
  } else {
    return y
  }
}

func runConvert(mimeBoundary string, inputJson string, tempDir string) {
  path := "/app/do-convert-stream-to-mime-multipart"
  args := make([]string, 3)
  args[0] = path
  args[1] = mimeBoundary
  args[2] = inputJson
  cmd := exec.Cmd {
    Path: path,
    Args: args,
    Dir: tempDir,
    Stdin: os.Stdin,
    Stderr: os.Stderr,
  }

  stdout, err := cmd.StdoutPipe()
  if err != nil {
    log.Fatalf("Could not open stdout for read: %s", err)
  }
  stdoutReader := bufio.NewReader(stdout)

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

  // Copy output from program until there is none left.
  //
  // This blocks exactly the right amount of time. Possibilities:
  //
  // * The program closes its stdout (usually by exiting).
  // * We get a signal and kill the program in a separate goroutine. It will
  //   exit soon, closing its stdout.
  wroteErrorOrDone := false
  wroteCloseDelimiter := false

  // DelimiterRegexp: finds done|error fragment or close-delimiter. (Those are
  // the only ones we care about.)
  DelimiterRegexp := regexp.MustCompile("\\r\\n--" + mimeBoundary + "(--|\\r\\n[Cc][Oo][Nn][Tt][Ee][Nn][Tt]-[Dd][Ii][Ss][Pp][Oo][Ss][Ii][Tt][Ii][Oo][Nn]\\s*:\\s*form-data;\\s*name=(\"done\"|\"error\"|done|error)\\r\\n\\r\\n)")
  BufferSize := 1024*1024               // how many bytes we read at a time
  MaxRemainderSize := 200               // max bytes cached between reads (to scan for a MIME boundary that spans reads)
  rawBuffer := make([]byte, BufferSize) // bytes from stdout.Read()
  // HACK: prepend \r\n to buffer. The first "--" from stdout might (nay,
  // *should*) mark the beginning of a boundary, and all future boundaries must
  // begin with \r\n, so this lets us simply "always scan for \r\n".
  rawBuffer[0] = '\r'
  rawBuffer[1] = '\n'
  remainderSize := 2                    // Bytes at start of rawBuffer: we'll scan them but not write them

  for {
    nBytes, err := stdoutReader.Read(rawBuffer[remainderSize:])
    if err == io.EOF {
      // nBytes == 0
      break
    }
    if err != nil {
      log.Fatalf("Error reading from %s: %v", path, err)
    }

    // usefulBuffer: bytes of process output we'll scan for delimiter, in two
    // parts:
    //
    // * 0:remainderSize => bytes we've already written to stdout but may
    //                      include a delimiter start
    // * remainderSize:len => bytes we haven't written to stdout
    usefulBuffer := rawBuffer[0:remainderSize + nBytes]
    for !wroteCloseDelimiter && len(usefulBuffer) > 0 {
      match := DelimiterRegexp.FindSubmatchIndex(usefulBuffer)

      if match == nil {
        // Write all to stdout, _including_ the remainder (which we'll re-scan
        // later). No need to delay writes.
        outputOrCrash(usefulBuffer[remainderSize:])

        // Move remainder to the start of the buffer, preparing for next read
        if cap(usefulBuffer) == cap(rawBuffer) && len(usefulBuffer) <= MaxRemainderSize {
          // we're already at the start
          remainderSize = len(usefulBuffer)
        } else {
          remainderSize = len(usefulBuffer)
          if remainderSize > MaxRemainderSize {
            remainderSize = MaxRemainderSize
          }

          copy(rawBuffer[:remainderSize], usefulBuffer[len(usefulBuffer) - remainderSize:])
        }

        usefulBuffer = nil // break out of loop
      } else {
        matchEndPos := match[1]

        if bytes.Equal([]byte("--"), usefulBuffer[match[2]:match[3]]) {
          // We found a close-delimiter, which marks the end of (valid) output
          //
          // Output the delimiter and then ignore all further output.

          outputOrCrash(usefulBuffer[remainderSize:matchEndPos])
          wroteCloseDelimiter = true
          remainderSize = 0
          usefulBuffer = nil

          // Error if we never saw "done" or "error"
          if !wroteErrorOrDone {
            fmt.Fprintf(os.Stderr, "%s did not output a 'done' or 'error' fragment", path)
            wroteErrorOrDone = true // do not output error later
            // It would have been nice to output the error message as part of
            // the stream instead of to stderr. Unfortunately, we can't because
            // by the time we started scanning usefulBuffer, we may have already
            // written some of it to stdout. TODO consider buffering the last
            // 200 bytes or so instead of outputting them instantaneously.
          }
        } else {
          // We found an "error" or "done" delimiter

          // output it...
          outputOrCrash(usefulBuffer[remainderSize:matchEndPos])
          wroteErrorOrDone = true

          // and then move usefulBuffer ahead, so we can scan again
          usefulBuffer = usefulBuffer[matchEndPos:]
          remainderSize = 0
        }
      }
    }
  }

  if err = cmd.Wait(); err != nil {
    if exiterr, ok := err.(*exec.ExitError); ok {
      // Buggy program exited with nonzero.
      if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
        if !wroteErrorOrDone {
          message := fmt.Sprintf("%s exited with status code %d", path, status.ExitStatus())
          printErrorAndExit(message, mimeBoundary)
        }
      } else {
        log.Fatalf("Could not determine exit code")
      }
    } else {
      log.Fatalf("convert-single-file failed: %s", err)
    }
  }

  if !wroteCloseDelimiter {
    if !wroteErrorOrDone {
      printErrorAndExit(path + " did not output a 'done' or 'error' fragment", mimeBoundary)
    } else {
      fmt.Fprintf(os.Stderr, "%s failed to output closing '\\r\\n--MIME-BOUNDARY--'", path)
      printCloseDelimiter(mimeBoundary)
    }
  }
}

func doConvert(mimeBoundary string, inputJson string, tempDir string) {
  prepareTempDir(tempDir)
  runConvert(mimeBoundary, inputJson, tempDir)
}

func main() {
  log.SetFlags(0)

  mimeBoundary := os.Args[1]
  inputJson := os.Args[2]
  tempDir := os.TempDir() + "/overview-convert-stream-to-mime-multipart"

  doConvert(mimeBoundary, inputJson, tempDir)
}
