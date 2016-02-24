#!/bin/sh
#
# USAGE: start.sh {region} {queue_name}

region=$1 ; shift
queue=$1 ; shift

dir=`pwd`

sqs-notify -region ${region} -daemon \
  -logfile "${dir}/demo.log" \
  -pidfile "${dir}/demo.pid" \
  ${queue} sqs-echo
