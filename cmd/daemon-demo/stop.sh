#!/bin/sh
#
# USAGE: stop.sh

kill `cat demo.pid`

rm -f demo.pid
