package main

import (
  "bytes"
  "fmt"
  "github.com/sergi/go-diff/diffmatchpatch"
  "image"
  "image/draw"
  "io"
  "io/ioutil"
  "log"
  "os"
  "os/exec"
  "path/filepath"
  "reflect"
  "regexp"
  "strings"
  "unicode/utf8"

  _ "image/png"
  _ "image/jpeg"
)

var PathsToTest = [...]string{
  "stdout",
  "stderr",
  "0.json",
  "0.blob",
  "0-thumbnail.png",
  "0-thumbnail.jpg",
  "0.txt",
}

var pdfDateRegex = regexp.MustCompile("  /(Creation|Mod)Date \\(D:\\d{14}")

func prepareTempDir(tempDir string, exampleDir string) {
  inputPath := exampleDir + "/input.blob"
  in, err := os.Open(inputPath)
  if err != nil {
    log.Fatalf("Failed to open %s for reading: %s", inputPath, err)
  }
  defer in.Close()

  outputPath := tempDir + "/input.blob"
  out, err := os.Create(outputPath)
  if err != nil {
    log.Fatalf("Failed to open %s for writing: %s", outputPath, err)
  }
  defer out.Close()

  if _, err := io.Copy(out, in); err != nil {
    log.Fatalf("Failed to write to %s: %s", outputPath, err)
  }
}

func runDoConvert(tempDir string, jsonString string) error {
  stdoutPath := tempDir + "/stdout"
  stdoutFile, err := os.Create(stdoutPath)
  if err != nil {
    log.Fatalf("Failed to open %s for writing: %s", stdoutPath, err)
  }
  defer stdoutFile.Close()

  path := "/app/do-convert-single-file"
  args := make([]string, 2)
  args[0] = path
  args[1] = jsonString
  cmd := exec.Cmd {
    Path: path,
    Args: args,
    Dir: tempDir,
    Stdout: stdoutFile,
    Stderr: os.Stderr,
  }

  return cmd.Run()
}

func describeDiffBetweenPdfBytes(filename string, expectedBytes []byte, actualBytes []byte) string {
  expectedBytesNorm := pdfDateRegex.ReplaceAll(expectedBytes, []byte("  /$1Date (D:20000000000000"))
  actualBytesNorm := pdfDateRegex.ReplaceAll(actualBytes, []byte("  /$1Date (D:20000000000000"))

  if bytes.Equal(expectedBytesNorm, actualBytesNorm) {
    return ""
  } else {
    return fmt.Sprintf("do-convert-single-file output wrong PDF in %s. (The test may be broken: different PDFs may be equivalent.)", filename)
  }
}

func describeDiffBetweenImages(filename string, expectedImage image.Image, actualImage image.Image) string {
  if expectedImage.Bounds() != actualImage.Bounds() {
    return fmt.Sprintf("do-convert-single-file output image %s with bounds %v, but we expected %v", filename, expectedImage.Bounds(), actualImage.Bounds())
  }

  if expectedImage.ColorModel() != actualImage.ColorModel() {
    return fmt.Sprintf("do-convert-single-file output image %s with color model %v, but we expected %v", filename, expectedImage.ColorModel(), actualImage.ColorModel())
  }

  expectedRgba := image.NewRGBA(expectedImage.Bounds())
  draw.Draw(expectedRgba, expectedRgba.Bounds(), expectedImage, image.Point { 0, 0 }, draw.Src)
  actualRgba := image.NewRGBA(actualImage.Bounds())
  draw.Draw(actualRgba, actualRgba.Bounds(), actualImage, image.Point { 0, 0 }, draw.Src)

  if !reflect.DeepEqual(expectedRgba.Pix, actualRgba.Pix) {
    return fmt.Sprintf("do-convert-single-file output image %s with wrong contents in", filename)
  }

  return ""
}

func describeDiffBetweenFiles(filename string, actualPath string, expectedPath string) string {
  expectedBytes, expectedErr := ioutil.ReadFile(expectedPath)
  actualBytes, actualErr := ioutil.ReadFile(actualPath)

  if os.IsNotExist(expectedErr) && !os.IsNotExist(actualErr) {
    return fmt.Sprintf("do-convert-single-file wrote %s, but we expected it not to exist", actualPath)
  } else if !os.IsNotExist(expectedErr) && os.IsNotExist(actualErr) {
    return fmt.Sprintf("do-convert-single-file did not write %s", actualPath)
  } else if os.IsNotExist(expectedErr) {
    return ""
  } else if utf8.Valid(expectedBytes) && !utf8.Valid(actualBytes) {
    return fmt.Sprintf("do-convert-single-file output invalid UTF-8 in %s", actualPath)
  } else if utf8.Valid(expectedBytes) {
    expectedString := strings.Trim(string(expectedBytes), " \r\n")
    actualString := strings.Trim(string(actualBytes), " \r\n")

    if expectedString == actualString {
      return ""
    } else {
      dmp := diffmatchpatch.New()
      diffs := dmp.DiffMain(expectedString, actualString, false)
      diffText := dmp.DiffPrettyText(diffs)
      return fmt.Sprintf("do-convert-single-file output wrong text in %s. Diff follows:\n%s", actualPath, diffText)
    }
  } else if bytes.Equal([]byte("%PDF"), expectedBytes[0:4]) {
    return describeDiffBetweenPdfBytes(actualPath, expectedBytes, actualBytes)
  } else if expectedImage, expectedFormat, err := image.Decode(bytes.NewReader(expectedBytes)); err == nil {
    actualImage, actualFormat, err := image.Decode(bytes.NewReader(actualBytes))
    if err != nil {
      return fmt.Sprintf("do-convert-single-file output a non-image in %s", actualPath)
    } else if expectedFormat != actualFormat {
      return fmt.Sprintf("do-convert-single-file output a %s image in %s; expected %s", actualFormat, actualPath, expectedFormat)
    } else {
      return describeDiffBetweenImages(actualPath, expectedImage, actualImage)
    }
  } else {
    if !bytes.Equal(expectedBytes, actualBytes) {
      return fmt.Sprintf("do-convert-single-file output wrong binary in %s. (The test may be broken: depending on the binary format, differing data may be equivalent.)", filename)
    } else {
      return ""
    }
  }
}

func testDoConvertOutputMatches(tempDir string, exampleDir string) string {
  for _, filename := range PathsToTest {
    errorMessage := describeDiffBetweenFiles(filename, tempDir + "/" + filename, exampleDir + "/" + filename)
    if errorMessage != "" {
      return errorMessage
    }
  }
  return ""
}

func testDoConvertSucceeds(tempDir string, exampleDir string) string {
  jsonPath := exampleDir + "/input.json"
  jsonBytes, err := ioutil.ReadFile(jsonPath)
  if err != nil {
    return fmt.Sprintf("Failed to read %s: %s", jsonPath, err)
  }

  if err := runDoConvert(tempDir, string(jsonBytes)); err != nil {
    return fmt.Sprintf("do-convert-single-file failed to run %s: %s", exampleDir, err)
  }

  return testDoConvertOutputMatches(tempDir, exampleDir)
}

func basename(dir string) string {
  parts := strings.Split(dir, "/")
  return parts[len(parts) - 1]
}

func indent(s string) string {
  return "    " + strings.Replace(s, "\n", "\n    ", -1)
}

func main() {
  testDirs, err := filepath.Glob("/app/test/test-*")
  if err != nil {
    log.Fatalf("Failed to read tests from /app/test/: %s", err)
  }

  // TAP test protocol: http://testanything.org/tap-specification.html
  fmt.Printf("1..%d\n", len(testDirs))

  gotFailure := false

  for testIndex, testDir := range testDirs {
    tempDir, err := ioutil.TempDir("", "test-do-convert-single-file")
    if err != nil {
      log.Fatalf("Could not create temporary directory for test: %s", err)
    }
    prepareTempDir(tempDir, testDir)
    testNumber := testIndex + 1
    testName := basename(testDir)
    diffDescription := testDoConvertSucceeds(tempDir, testDir)
    if diffDescription == "" {
      fmt.Printf("ok %d - %s\n", testNumber, testName)
    } else {
      gotFailure = true
      fmt.Printf("not ok %d - %s\n%s\n", testNumber, testName, indent(diffDescription))
    }
  }

  if gotFailure {
    os.Exit(1)
  }
}
