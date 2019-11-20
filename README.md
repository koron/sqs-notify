# SQS notify

[![CircleCI](https://circleci.com/gh/koron/sqs-notify.svg?style=svg)](https://circleci.com/gh/koron/sqs-notify)
[![Go Report Card](https://goreportcard.com/badge/github.com/koron/sqs-notify)](https://goreportcard.com/report/github.com/koron/sqs-notify)

Listen a SQS queue, execute a command when received.  A message body is passed
as STDIN to the command.

For old version (v1), check [doc/v1.md](./doc/v1.md).

## Installation

Install and upgrade.

```console
$ go get -u github.com/koron/sqs-notify/cmd/sqs-notify2
```

## Environment variables

*   `AWS_SHARED_CREDENTIALS_FILE` - Shared credentials file path can be set to
    instruct the SDK to use an alternate file for the shared credentials. If
    not set the file will be loaded from $HOME/.aws/credentials on Linux/Unix
    based systems, and %USERPROFILE%\.aws\credentials on Windows.

## Options

From online help.

```
  -cache string
    	cache name or connection URL
    	 * memory://?capacity=1000
    	 * redis://[{USER}:{PASS}@]{HOST}/[{DBNUM}]?[{OPTIONS}]
    	
    	   DBNUM: redis DB number (default 0)
    	   OPTIONS:
    		* lifetime : lifetime of cachetime (ex. "10s", "2m", "3h")
    		* prefix   : prefix of keys
    	
    	   Example to connect the redis on localhost: "redis://:6379"
  -createqueue
    	create queue if not exists
  -endpoint string
    	Endpoint of SQS
  -logfile string
    	log file path
  -max-retries int
    	max retries for AWS
  -multiplier value
    	pooling the SQS in multiple runner (default 1)
  -pidfile string
    	PID file path (require -logfile)
  -profile string
    	AWS profile name
  -queue value
    	SQS queue name
  -region string
    	AWS region (default "us-east-1")
  -remove-policy value
    	policy to remove messages from SQS
    	 * succeed          : after execution, succeeded (default)
    	 * ignore_failure   : after execution, ignore its result
    	 * before_execution : before execution (default succeed)
  -timeout duration
    	timeout for command execution (default 0 - no timeout)
  -version
    	show version
  -wait-time-seconds int
    	wait time in seconds for next polling. (default -1, disabled, use queue default) (default -1)
  -workers int
    	num of workers (default 16)
```

## Guide

Basic usage:

    sqs-notify2 [-region {region}] -queue {queue name} {command and args}

1.  Prepare AWS auth information.
    1.  Use `~/.aws/credentials` (recomended).

        sqs-notify supports `~/.aws/credentials` profiles.
        `-profile` option can choose the profile used to.  Example:

        ```ini
        [my_sqs]
        aws_access_key_id=foo
        aws_secret_access_key=bar
        ```

        ```console
        $ sqs-notify2 -profile my_sqs ...
        ```

    2.  Use two environment variables.
        *   `AWS_ACCESS_KEY_ID`
        *   `AWS_SECRET_ACCESS_KEY`

2.  Run sqs-notify

    ```console
    $ sqs-notify2 -queue my_queue cat
    ```

    This example just copy messages to STDOUT.  If you want to access the queue
    via ap-northeast-1 region, use below command.

    ```console
    $ sqs-notify2 -region ap-northeast-1 -queue my_queue cat
    ```

### Name of regions

*   `us-east-1` (default)
*   `us-west-1`
*   `us-west-2`
*   `eu-west-1`
*   `ap-southeast-1`
*   `ap-southeast-2`
*   `ap-northeast-1`
*   `sp-east-1`

### Logging

When `-logfile {FILE PATH}` is given, all messages which received are logged
into the file.  If FILE PATH is `-`, it output all logs to STDOUT not file.

Using `-pidfile {FILE PATH}` with `-logfile`, sqs-notify writes own PID to the
file.  You can send SIGHUP to that PID to rotate log.

## Miscellaneous

### LF at EOF

When message doesn't have LF at EOF (end of file/message), the last line can't
be handled by `read` shell command or so.  This is limitation of `read`
command, not sqs-notify.  Therefore this kind of scripts don't work correctly
for messages without LF at EOF:

```sh
#!/bin/sh
while read line
do
  echo "received: $line"
done
```

To work around this problem, use `xargs` like this.

```sh
#!/bin/sh
xargs -0 echo | (
while read line
do
  echo "received: $line"
done
)
```

### /dev/stdin

You can use `/dev/stdin` pseudo file, if your system support it, like this:

```sh
#!/bin/sh

data=`cat /dev/stdin`

# do something for data.
```

### sqs-echo

sqs-echo is useful for debugging received SQS message with sqs-notify.  It just
shows date, time, byte num and contents of received messages.  Example output
is below:

```
2015/05/07 12:43:08 (7) "foo\nbar"
2015/05/07 12:43:12 (3) "qux"
```

You can install sqs-echo with below command.

```
$ go install github.com/koron/sqs-notify/cmd/sqs-echo
```

## LICENSE

MIT License.  See [LICENSE](./LICENSE) for details.
