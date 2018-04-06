#!/bin/sh

read -r first_line
first_line=$(echo "$first_line" | tr -d '\r')
while read -r header_line; do
  header_line=$(echo "$header_line" | tr -d '\r')
  if [ -z "$header_line" ]; then
    # Empty \r\n: now we're at the body
    break
  fi
done

echo "Handling: $first_line" >>/tmp/log

if echo "$first_line" | grep -q 'POST /Task/'; then
  echo -en 'HTTP/1.1 202 Accepted\r\n\r\n'
  cat - > /tmp/run-test/posted-data
elif echo "$first_line" | grep -q 'POST /Task'; then
  <&-
  if [ -f /tmp/run-test/task ]; then
    echo -en 'HTTP/1.1 200 OK\r\n\r\n'
    cat /tmp/run-test/task
  else
    echo -en 'HTTP/1.1 204 No Content\r\n\r\n'
  fi
elif echo "$first_line" | grep -q 'GET /blob'; then
  <&-
  if [ -f /tmp/run-test/blob ]; then
    echo -en 'HTTP/1.1 200 OK\r\n\r\n'
    cat /tmp/run-test/blob
  else
    echo -en 'HTTP/1.1 204 No Content\r\n\r\n'
  fi
else
  echo -en 'HTTP/1.1 404 Not Found\r\n\r\n'
fi
