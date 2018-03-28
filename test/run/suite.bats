#!/usr/bin/env bats

TEST_DIR=/go/src/app/test/run
cmd=/go/src/app/bin/run

setup() {
  [ -d /tmp/run-test ] && rm -r /tmp/run-test
  mkdir /tmp/run-test
  pushd /tmp/run-test
  nc -lk -p 8080 -e "$TEST_DIR/server.sh" &
  sleep 0.3 # wait to listen on port
}

teardown() {
  kill %1
  sleep 0.3 # wait to free port
  popd
  rm -rf /tmp/run-test
}

set_convert() {
  [ -d /app ] || mkdir -p /app
  echo "#!/bin/sh" > /app/convert
  echo "$1" >> /app/convert
  chmod +x /app/convert
}

set_task() {
  echo -n "$1" >/tmp/run-test/task
}

set_blob() {
  echo -n "$1" >/tmp/run-test/blob
}

run_tick() {
  POLL_URL="http://localhost:8080/Task" "$cmd" just-one-tick
}

@test "set arguments and stream blob" {
  set_convert 'echo -n "$1" > /tmp/run-test/input.boundary; echo -n "$2" > /tmp/run-test/input.json; cat - > /tmp/run-test/input.blob'
  set_task '{"url":"http://localhost:8080/Task/id","blob":{"url":"http://localhost:8080/blob"}}'
  set_blob 'Some blob'
  run_tick
  diff -u /tmp/run-test/task /tmp/run-test/input.json
  diff -u /tmp/run-test/blob /tmp/run-test/input.blob
}

@test "pipe chunked output to HTTP server" {
  set_convert 'cat - >/dev/null; echo -n OUTPUT'
  set_task '{"url":"http://localhost:8080/Task/id","blob":{"url":"http://localhost:8080/blob"}}'
  set_blob 'Some blob'
  run_tick
  echo -en '6\r\nOUTPUT\r\n0\r\n\r\n' > /tmp/run-test/expect-posted-data
  diff -u /tmp/run-test/expect-posted-data /tmp/run-test/posted-data
}

@test "succeed if connection fails" {
  kill -9 %1
  run_tick
}

@test "succeed on 204 No Content" {
  run_tick
}
