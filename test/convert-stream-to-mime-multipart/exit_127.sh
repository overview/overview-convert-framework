#!/bin/sh

echo -en "--$1\r\nContent-Disposition: form-data; name=0.json\r\n\r\n"
echo -n "$2"
echo -en "\r\n--$1\r\nContent-Disposition: form-data; name=0.blob\r\n\r\n"
cat

exit 127
