#!/usr/bin/env bats

TEST_DIR=/go/src/app/test/convert-stream-to-mime-multipart
cmd=/go/src/app/bin/convert-stream-to-mime-multipart

set_convert_script() {
	[ -d /app ] || mkdir /app
  cp "$TEST_DIR"/"$1".sh /app/do-convert-stream-to-mime-multipart
}

input_blob() {
  echo -n 'blob'
}

input_json() {
  echo -n '{"blob":{"nBytes":4}}'
}

@test "output program output" {
  set_convert_script echo
  input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/simple-out.mime -
}

@test "delete files between invocations" {
	set_convert_script echo_with_garbage
	input_blob | $cmd MIME-BOUNDARY $(input_json) >/dev/null
  [ -f /tmp/*/garbage ]
  set_convert_script echo
	input_blob | $cmd MIME-BOUNDARY $(input_json) >/dev/null
  [ ! -f /tmp/*/garbage ]
}

@test "output error if /app/do-convert-stream-to-mime-multipart does not exist" {
  rm -f /app/do-convert-stream-to-mime-multipart
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/error-no-code.mime -
}

@test "output error if /app/do-convert-stream-to-mime-multipart is not executable" {
  set_convert_script error_not_executable
  rm -f /app/do-convert-stream-to-mime-multipart
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/error-not-executable.mime -
}

@test "output error if script exits with nonzero status code" {
  # This is what an out-of-memory error looks like
	set_convert_script exit_127
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/error-bad-exit-code.mime -
}

@test "output quickly on SIGINT" {
	set_convert_script interrupt_parent_then_wait
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/cancel.mime -
}

@test "add close-delimiter" {
  set_convert_script error_no_close_delimiter
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/simple-out.mime -
}

@test "add error and close-delimiter" {
  set_convert_script error_no_output
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/error-no-close-delimiter.mime -
}
