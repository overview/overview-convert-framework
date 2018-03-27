#!/usr/bin/env bats

TEST_DIR=/go/src/app/test/convert-single-file
cmd=/go/src/app/bin/convert-single-file

setup() {
	[ -d /tmp/convert-single-file-test ] && rm -r /tmp/convert-file-single-test
	mkdir /tmp/convert-file-single-test
	pushd /tmp/convert-file-single-test
}

teardown() {
	popd
	rm -rf /tmp/convert-file-single-test
}

serve_http() {
	echo -e "$1" | nc -l -p 8080 >/dev/null &
	sleep 0.3 # wait for nc to start listening
}

set_convert_script() {
	[ -d /app ] || mkdir /app
	echo '#!/bin/sh' > /app/do-convert-single-file
	echo "$1" >> /app/do-convert-single-file
	chmod +x /app/do-convert-single-file
}

set_input_json() {
	echo '{"url":"http://localhost:8080/anything"}' > input.json
}

@test "download input.blob" {
	serve_http 'HTTP/1.1 200 OK\r\n\r\nblob'
	set_input_json
	run $cmd MIME-BOUNDARY
	[ -f input.blob ]
	[ "$(cat input.blob)" = 'blob' ]
}

@test "output 0.json+0.blob+done" {
	serve_http 'HTTP/1.1 200 OK\r\n\r\nblob'
	set_input_json
	set_convert_script 'echo -n 42 > 0.json; echo -n bar > 0.blob'
	$cmd MIME-BOUNDARY | diff -u "$TEST_DIR"/simple-out.mime -
}

@test "output error if 0.blob does not exist" {
	serve_http 'HTTP/1.1 200 OK\r\n\r\nblob'
	set_input_json
	set_convert_script 'echo -n 42 > 0.json'
	$cmd MIME-BOUNDARY | diff -u "$TEST_DIR"/error-no-blob.mime -
}

@test "output error if script exits with nonzero status code" {
	serve_http 'HTTP/1.1 200 OK\r\n\r\nblob'
	set_input_json
	set_convert_script 'echo -n 42 > 0.json; exit 127'
	$cmd MIME-BOUNDARY | diff -u "$TEST_DIR"/error-bad-exit-code.mime -
}

@test "output progress and error events" {
	serve_http 'HTTP/1.1 200 OK\r\n\r\nblob'
	set_input_json
	set_convert_script 'echo c1/5; echo b20/100; echo 0.523; echo foo'
	$cmd MIME-BOUNDARY | diff -u "$TEST_DIR"/progress-and-error.mime -
}
