# SQS notify

[![CircleCI](https://circleci.com/gh/koron/sqs-notify.svg?style=svg)](https://circleci.com/gh/koron/sqs-notify)
[![Go Report Card](https://goreportcard.com/badge/github.com/koron/sqs-notify)](https://goreportcard.com/report/github.com/koron/sqs-notify)

Listen a SQS queue, execute a command when received.  A message body is passed
as STDIN to the command.

(in Japanese) SQS を監視しメッセージを取得したら指定のコマンドを実行します。
メッセージの内容はコマンドの標準入力として渡します。

See [sqs-notify2](#sqs-notify2) for new version.

## Installation

### Pre built binaries

Please check below URL for pre built binaries:

<https://github.com/koron/sqs-notify/releases/latest>

### Build by yourself

```
$ go get github.com/koron/sqs-notify
```

or update

```
$ go get -u github.com/koron/sqs-notify
```

Require git command for dependencies.

### for developer

```
$ go get github.com/goamz/goamz
```


## Usage

From online help.

```
Usage: sqs-notify [OPTIONS] {queue name} {command and args...}

OPTIONS:
  -daemon
        run as a daemon
  -ignorefailure
        Don't care command failures
  -logfile string
        Log file path
  -messagecount int
        retrieve multiple messages at once (default 10)
  -mode string
        pre-defined set of options for specific usecases
  -msgcache int
        Num of last messages in cache
  -nowait
        Don't wait end of command
  -pidfile string
        PID file path (require -logfile)
  -redis string
        Use redis as messages cache
  -region string
        AWS Region for queue (default "us-east-1")
  -retrymax int
        Num of retry count (default 4)
  -version
        show version
  -worker int
        Num of workers (default 4)

Source: https://github.com/koron/sqs-notify
```

### Guide

Basic usage:

    sqs-notify [-region {region}] {queue name} {command and args}

1.  Prepare AWS auth information.
    1.  Use `~/.aws/credentials` (recomended).

        sqs-notify supports `~/.aws/credentials` file, and use info from
        `sqs-notify` or `default` section in this order.  Example:

        ```ini
        [sqs-notify]
        aws_access_key_id=foo
        aws_secret_access_key=bar
        ```

    2.  Use two environment variables.
        *   `AWS_ACCESS_KEY_ID`
        *   `AWS_SECRET_ACCESS_KEY`
2.  Run sqs-notify

    ```
    $ sqs-notify your-queue cat
    ```

    This example just copy messages to STDOUT.  If you want to access the queue
    via ap-northeast-1 region, use below command.

    ```
    $ sqs-notify -region ap-northeast-1 your-queue cat
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

Log example:

```
2014/05/28 00:13:29 EXECUTED queue:kaoriya body:"abc\n" cmd:cat status:0
2014/05/28 00:14:27 EXECUTED queue:kaoriya body:"def\n" cmd:cat status:0
2014/05/28 00:14:54 EXECUTED queue:kaoriya body:"foo\nbar\n" cmd:cat status:0
```

Each items in one log are separated by tab char (`\t`).


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

### Suppress for duplicated message

To suppress executions of the command for duplicated (seen recently) messages,
please use `-msgcache {num}` option.  `{num}` means cache size for messages.

Below command shows how to enable suppression with cache size 10.

    $ sqs-notify -msgcache 10 -logfile - my-queue sqs-echo

You'll get log like this when sqs-notify detect duplicated message.

```
2015/05/07 12:13:08 EXECUTED queue:my-queue body:"foo\n" cmd:cat status:0
2015/05/07 12:43:11 SKIPPED  queue:my-queue body:"foo\n"
```

sqs-notify stores hashs (MD5) of message to detect duplication.

#### Use redis to suppress duplicated messages

sqs-notify では `-redis` オプションに設定ファイル(JSON)を指定すると、メッセージ
ハッシュの保存先をRedisに変更できます。

実行例:

    $ sqs-notify -redis redis-example.json -logfile - my-queue sqs-echo

JSON内で使用できる設定項目は以下のとおりです。

Name       |Mandatory?  |Description
-----------|------------|------------
addr       |YES         |"host:port" address
keyPrefix  |YES         |prefix of keys
expiration |YES         |time to expiration for keys (acceptable [format](http://golang.org/pkg/time/#ParseDuration))
network    |NO          |"tcp" or "udp" (default "tcp")
password   |NO          |password to connect redis
maxRetries |NO          |default: no retry

[redis-example.json](./redis-example.json) には最小限の設定が記載されています。

### at-most-once mode

**at-most-once** mode provides at-most-once command execution.

    $ sqs-notify -mode at-most-once -msgcache 10 my-queue sqs-echo

This mode implies `-ignorefailure` option, excludes `-nowait` option, and
requires one of `-msgcache` or `-redis` option.

## sqs-notify2

New version of sqs-noitfy (v2).
It is an experimental for now.

### Installation and upgrade

```console
$ go get -u github.com/koron/cmd/sqs-notify2
```

### Environment variables for v2

*   `AWS_SHARED_CREDENTIALS_FILE` - Shared credentials file path can be set to
    instruct the SDK to use an alternate file for the shared credentials. If
    not set the file will be loaded from $HOME/.aws/credentials on Linux/Unix
    based systems, and %USERPROFILE%\.aws\credentials on Windows.

### Options for v2

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
  -logfile string
    	log file path
  -max-retries int
    	max retries for AWS
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
    	 * before_execution : before execution
  -timeout duration
    	timeout for command execution (default 0 - no timeout)
  -version
    	show version
  -workers int
    	num of workers (default 8)
```

## LICENSE

MIT License.  See [LICENSE](./LICENSE) for details.
