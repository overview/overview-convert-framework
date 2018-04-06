#!/bin/sh

do_job() {
  sleep 3
  echo -en "\r\n--$1\r\nContent-Disposition: form-data; name=BAD\r\n\r\n\r\n--$1--"
}

echo -en "--$1\r\nContent-Disposition: form-data; name=0.json\r\n\r\n"
echo -n "$2"
echo -en "\r\n--$1\r\nContent-Disposition: form-data; name=0.blob\r\n\r\n"
cat
do_job &
trap "kill %1" INT
kill -INT $(grep PPid /proc/$$/status | cut -f2)
wait %1 || true
