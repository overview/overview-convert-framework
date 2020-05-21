#!/usr/bin/env bats

TEST_DIR=/go/src/app/test/run
cmd=/go/src/app/bin/run

setup() {
  [ -d /tmp/run-test ] && rm -r /tmp/run-test
  mkdir /tmp/run-test
  pushd /tmp/run-test

  # Create a simple HTTP server to represent Overview's side of things

  # POST /Task
  # If `set_task` was called: return 201 Created with the task data
  # Otherwise: return 204 No Content (meaning "no task")
  cat > /tmp/run-test/create-task.sh <<'EOF'
#!/bin/sh -e
[ "$REQUEST_METHOD" = "POST" ]
if [ -f /tmp/run-test/task ]; then
  echo -en 'HTTP/1.1 201 Created\r\n\r\n'
  cat /tmp/run-test/task
else
  echo -en 'HTTP/1.1 204 No Content\r\n\r\n'
fi
EOF

  # POST /Task/id
  # Write request body to /tmp/run-test/posted-data and return 202 Accepted
  cat > /tmp/run-test/post-task.sh <<'EOF'
#!/bin/sh -e
[ "$REQUEST_METHOD" = "POST" ]
echo "Transfer-Encoding: $HTTP_TRANSFER_ENCODING" > /tmp/run-test/posted-data
cat - >> /tmp/run-test/posted-data
echo -en 'HTTP/1.1 202 Accepted\r\n\r\n'
EOF

  # GET /healthz
  # Return 200 OK if the server is up
  touch /tmp/run-test/healthz

  # POST /TaskWithBrokenPost/id
  # Kill lighttpd, resetting the TCP connection
  cat > /tmp/run-test/broken-post-task.sh <<'EOT'
#!/bin/sh
killall -9 lighttpd
sleep 1  # wait for connection to die
EOT

  # Other endpoints we create using flat files:
  #
  # GET /blob
  # If `set_blob` was called: return 200 OK with the contents
  # Otherwise: return 404 Not Found

  cat > /tmp/run-test/lighttpd.conf <<'EOF'
server.port = 8080
server.bind = "127.0.0.1"
server.document-root = "/var/www/html"
server.breakagelog = "/dev/stderr"
server.errorlog = "/dev/stderr"
server.modules = ( "mod_cgi", "mod_setenv", "mod_alias", "mod_accesslog", "mod_staticfile" )
server.stream-request-body = 0
accesslog.filename = "/dev/stderr"
cgi.assign = ( ".sh" => "/bin/sh" )

$REQUEST_HEADER["content-length"] == "" {
  # TL;DR indicate to CGI scripts whether the HTTP request was chunked
  #
  # It's complicated....
  #
  # CGI doesn't allow "Transfer-Encoding: chunked" in requests, because the HTTP
  # server is meant to decode chunked requests. And for some reason, lighttpd
  # doesn't expose the "Transfer-Encoding" HTTP header. (Maybe because lighttpd
  # doesn't _really_ support chunked requests: it buffers them before any
  # modules can see them, because modules depend on content-length.)
  #
  # Luckily, "content-length" happens to be unset when Transfer-Encoding is
  # "chunked". (The HTTP headers are mutually exclusive.) So if there's no
  # Content-Length, assume the request is chunked.
  #
  # Then we set an environment variable that the CGI script can read.
  setenv.set-environment = ( "HTTP_TRANSFER_ENCODING" => "chunked" )
}

alias.url = (
    "/healthz" => "/tmp/run-test/healthz",
    "/Task/id" => "/tmp/run-test/post-task.sh",
    "/TaskWithBrokenPost/id" => "/tmp/run-test/broken-post-task.sh",
    "/Task" => "/tmp/run-test/create-task.sh",
    "/blob" => "/tmp/run-test/blob" )
EOF

  lighttpd -f /tmp/run-test/lighttpd.conf # default is to daemonize
  while ! wget -q -O /dev/null "http://localhost:8080/healthz"; do
    # wait to listen on port
    sleep 0.01
  done
}

teardown() {
  killall lighttpd 2>/dev/null || true  # if lighttpd is already dead, that's okay
  sleep 1 # wait for port 8080 to become free
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
  echo -en 'Transfer-Encoding: chunked\nOUTPUT' > /tmp/run-test/expect-posted-data
  diff -u /tmp/run-test/expect-posted-data /tmp/run-test/posted-data
}

@test "succeed if connection fails" {
  killall lighttpd
  sleep 1 # wait for port 8080 to become free
  run_tick
}

@test "succeed if POST fails" {
  set_convert 'cat - >/dev/null; echo -n OUTPUT'
  set_task '{"url":"http://localhost:8080/TaskWithBrokenPost/id","blob":{"url":"http://localhost:8080/blob"}}'
  set_blob 'Some blob'
  run run_tick
  [ "$status" -eq 0 ]
  # [ "${output##*:}" = ' connection reset by peer' ]
  [ "${output##*:}" = ' EOF' ]
}

@test "succeed if DNS resolve fails" {
  POLL_URL="http://nonexistent-hostname.test:8080/Task" "$cmd" just-one-tick
}

@test "succeed on 204 No Content" {
  run_tick
}
