# Run sqs-notify2 as Service

This document describes "How make sqs-notify2 work as service" on Ubuntu.
Some paths or commands in this document should be modified for other Linux
distributions.

## common preparations

Build and install sqs-notify2:

```console
$ mkdir -p /opt/sqsnotify
$ go build -v ./cmd/sqs-notify2
$ sudo cp ./sqs-notify2 /opt/sqsnotify
```

(OPTION) Install message processing command:

```console
$ go build ./cmd/sqs-echo
$ sudo cp ./sqs-echo /opt/sqsnotify
```

Create `/opt/sqsnotify/credentilas` for AWS credentials:

```
[default]
aws_access_key_id=XXXXXXXXXXXXXXXXXXXX
aws_secret_access_key=XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
```

Create `/opt/sqsnotify/env` file to use above credentials file.

```
AWS_SHARED_CREDENTIALS_FILE=/opt/sqsnotify/credentials
```

## using systemd

Then create a service file `/etc/systemd/system/sqsnotify.service` to define a
service unit.

```
[Unit]
Description = sqs-notify daemon

[Service]
ExecStart = /opt/sqsnotify/sqs-notify2 -region {YOUR_REGION} -queue {YOUR_QUEUE} [OPTIONS] {YOUR_COMMAND} [YOUR_COMMAND_OPTIONS]
Restart = always
EnvironmentFile = /opt/sqsnotify/env
Type = simple

[Install]
WantedBy = multi-user.target
```

Enable (install) sqsnotify service unit:

```console
sudo systemctl enable sqsnotify
```

Start the service:

```console
sudo systemctl start sqsnotify
```

Check staus of the service:

```console
$ sudo systemctl status sqsnotify
```

See log (journal) of the service:

```console
$ sudo journalctl -u sqsnotify
```

Stop the service:

```console
$ sudo systemctl stop sqsnotify
```

Restart the service:

```console
$ sudo systemctl restart sqsnotify
```

## using supervisord

Install supervisor.

```console
$ sudo apt install supervisor
```

Create a service file `/etc/supervisor/conf.d/sqsnotify.conf` to define the
sqs-notify service.

```
[program:sqsnotify]
command = /opt/sqsnotify/sqs-notify2 -region {YOUR_REGION} -queue {YOUR_QUEUE} [OPTIONS] {YOUR_COMMAND} [YOUR_COMMAND_OPTIONS]
directory = /opt/sqsnotify
process_name = sqsnotify
numprocs = 1
autostart = true
autorestart = true
environment = AWS_SHARED_CREDENTIALS_FILE="/opt/sqsnotify/credentials"
```

Restart (or start) supervisor service to start sqsnotify.

```console
$ sudo systemctl restart supervisor
```

Check staus of sqsnotify:

```console
$ sudo supervisorctl status sqsnotify
```

Logs from sqsnotify are stored in `/var/log/supervisor/sqsnotify-std*.log`.

See <http://supervisord.org/> for more details of supervisord.
