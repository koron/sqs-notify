package main

import (
	"errors"
	"flag"
	"github.com/koron/sqs-notify/sqsnotify"
	"launchpad.net/goamz/aws"
)

type config struct {
	region   string
	worker   int
	nowait   bool
	retryMax int
	logfile  string
	pidfile  string
	queue    string
	cmd      string
	args     []string
}

func getConfig() (*config, error) {
	var region string
	var worker int
	var nowait bool
	var retryMax int
	var logfile string
	var pidfile string

	flag.StringVar(&region, "region", "us-east-1", "AWS Region for queue")
	flag.IntVar(&worker, "worker", 4, "Num of workers")
	flag.BoolVar(&nowait, "nowait", false, "Didn't wait end of command")
	flag.IntVar(&retryMax, "retrymax", 4, "Num of retry count")
	flag.StringVar(&logfile, "logfile", "", "Log file path")
	flag.StringVar(&pidfile, "pidfile", "", "PID file path (require -logfile)")
	flag.Parse()

	// Parse arguments.
	args := flag.Args()
	if len(args) < 2 {
		usage()
	}

	return &config{
		region:   region,
		worker:   worker,
		nowait:   nowait,
		retryMax: retryMax,
		logfile:  logfile,
		pidfile:  pidfile,
		queue:    args[0],
		cmd:      args[1],
		args:     args[2:],
	}, nil
}

func (c *config) toApp() (*app, error) {
	// Retrieve an AWS auth.
	auth, err := aws.EnvAuth()
	if err != nil {
		return nil, err
	}

	// Determine a region.
	region, ok := aws.Regions[c.region]
	if !ok {
		return nil, errors.New("unknown region:" + c.region)
	}

	// logfile and pidfile.
	if len(pidfile) > 0 && len(logfile) == 0 {
		return nil, errors.New("`-pidfile' requires `-logfile' option")
	}

	notify := sqsnotify.New(auth, region, c.queue)

	return &app{auth, region, c.worker, c.nowait, c.retryMax,
		notify, c.cmd, c.args}, nil
}
