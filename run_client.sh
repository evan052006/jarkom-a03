#!/bin/sh
go run ./client -host 127.0.0.1 -port 8056 -uri /
# go run ./client -host 127.0.0.1 -port 12345 \
#   -uri '/greet/2306123123?name=Rani' -accept application/xml
# go run ./client -host 127.0.0.1 -port 12345 \
#   -uri /greet/2306123123 \
#   -accept 'application/xml;q=0.8, application/json;q=0.9' -encoding gzip
