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
  echo "$1" >/tmp/run-test/task
}

set_blob() {
  echo "$1" >/tmp/run-test/blob
}

run_tick() {
  POLL_URL="http://localhost:8080/Task" "$cmd" just-one-tick
}

@test "stream input blob" {
  set_convert 'cat - > /tmp/run-test/input.blob'
  set_task '{"url":"http://localhost:8080/Task/id","blob":{"url":"http://localhost:8080/blob"}}'
  set_blob 'Some blob'
  run_tick
}
