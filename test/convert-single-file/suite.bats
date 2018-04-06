#!/usr/bin/env bats

TEST_DIR=/go/src/app/test/convert-single-file
cmd=/go/src/app/bin/convert-single-file

set_convert_script() {
	[ -d /app ] || mkdir /app
	echo '#!/bin/sh' > /app/do-convert-single-file
	echo "$1" >> /app/do-convert-single-file
	chmod +x /app/do-convert-single-file
}

input_blob() {
  echo -n 'blob'
}

input_json() {
  echo '{"blob":{"nBytes":4}}'
}

@test "output 0.json+0.blob+done" {
	set_convert_script 'echo -n 42 > 0.json; echo -n bar > 0.blob'
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/simple-out.mime -
}

@test "output 0-thumbnail.png+0-thumbnail.jpg+0.txt" {
	set_convert_script 'echo -n 42 > 0.json; echo -n bar > 0.blob; echo -n txt > 0.txt; echo -n png > 0-thumbnail.png; echo -n jpg > 0-thumbnail.jpg'
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/complete-out.mime -
}

@test "delete files between invocations" {
	set_convert_script 'echo -n 42 > 0.json; echo -n bar > 0.blob; echo -n txt > 0.txt; echo -n png > 0-thumbnail.png; echo -n jpg > 0-thumbnail.jpg'
	input_blob | $cmd MIME-BOUNDARY $(input_json) >/dev/null
  set_convert_script 'echo -n 42 > 0.json; echo -n bar > 0.blob'
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/simple-out.mime -
}

@test "output error if input stream has wrong length" {
	echo -n 'a--' | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/error-truncated-input.mime -
}

@test "output error if /app/do-convert-single-file does not exist" {
  rm -f /app/do-convert-single-file
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/error-no-code.mime -
}

@test "output error if 0.blob does not exist" {
	set_convert_script 'echo -n 42 > 0.json'
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/error-no-blob.mime -
}

@test "output error if script exits with nonzero status code" {
	set_convert_script 'echo -n 42 > 0.json; exit 127'
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/error-bad-exit-code.mime -
}

@test "output progress and error events" {
	set_convert_script 'echo c1/5; echo b20/100; echo 0.523; echo foo'
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/progress-and-error.mime -
}

@test "output quickly on SIGINT" {
	set_convert_script 'do_job() { sleep 3; echo bad-success > 0.json; echo bad-success > 0.blob; }; trap "kill %1" INT; echo c1/5; do_job & kill -INT $(grep PPid /proc/$$/status | cut -f2); do_job & wait %1 || true'
	input_blob | $cmd MIME-BOUNDARY $(input_json) | diff -u "$TEST_DIR"/cancel.mime -
}
