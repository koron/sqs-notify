#!/bin/sh
#
# USAGE: rotate.sh

test -f demo.9.log && rm -f demo.9.log
test -f demo.8.log && mv demo.8.log demo.9.log
test -f demo.7.log && mv demo.7.log demo.8.log
test -f demo.6.log && mv demo.6.log demo.7.log
test -f demo.5.log && mv demo.5.log demo.6.log
test -f demo.4.log && mv demo.4.log demo.5.log
test -f demo.3.log && mv demo.3.log demo.4.log
test -f demo.2.log && mv demo.2.log demo.3.log
test -f demo.1.log && mv demo.1.log demo.2.log
test -f demo.log && mv demo.log demo.1.log

kill -HUP `cat demo.pid`
