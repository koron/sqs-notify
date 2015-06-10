package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/goamz/goamz/aws"
	"github.com/koron/hupwriter"
	"github.com/koron/sqs-notify/awsutil"
	"github.com/koron/sqs-notify/sqsnotify"
)

type config struct {
	region   string
	worker   int
	nowait   bool
	retryMax int
	msgcache int
	redis    string
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
	var msgcache int
	var redis string
	var logfile string
	var pidfile string

	flag.StringVar(&region, "region", "us-east-1", "AWS Region for queue")
	flag.IntVar(&worker, "worker", 4, "Num of workers")
	flag.BoolVar(&nowait, "nowait", false, "Didn't wait end of command")
	flag.IntVar(&retryMax, "retrymax", 4, "Num of retry count")
	flag.IntVar(&msgcache, "msgcache", 0, "Num of last messages in cache")
	flag.StringVar(&redis, "redis", "", "Use redis as messages cache")
	flag.StringVar(&logfile, "logfile", "", "Log file path")
	flag.StringVar(&pidfile, "pidfile", "", "PID file path (require -logfile)")
	flag.Parse()

	// Parse arguments.
	args := flag.Args()
	if len(args) < 2 {
		usage()
	}

	if len(pidfile) > 0 && len(logfile) == 0 {
		return nil, errors.New("`-pidfile' requires `-logfile' option")
	}

	return &config{
		region:   region,
		worker:   worker,
		nowait:   nowait,
		retryMax: retryMax,
		msgcache: msgcache,
		redis:    redis,
		logfile:  logfile,
		pidfile:  pidfile,
		queue:    args[0],
		cmd:      args[1],
		args:     args[2:],
	}, nil
}

func (c *config) toApp() (*app, error) {
	// Retrieve an AWS auth.
	auth, err := awsutil.GetAuth("sqs-notify")
	if err != nil {
		return nil, err
	}

	// Determine a region.
	region, ok := aws.Regions[c.region]
	if !ok {
		return nil, errors.New("unknown region:" + c.region)
	}

	// logfile and pidfile.
	var l *log.Logger
	if len(c.logfile) > 0 {
		if c.logfile == "-" {
			l = log.New(os.Stdout, "", log.LstdFlags)
		} else {
			w := hupwriter.New(c.logfile, c.pidfile)
			l = log.New(w, "", log.LstdFlags)
		}
	}

	notify := sqsnotify.New(auth, region, c.queue)

	jobs, err := c.newJobs()
	if err != nil {
		return nil, err
	}

	return &app{
		logger:   l,
		auth:     auth,
		region:   region,
		worker:   c.worker,
		nowait:   c.nowait,
		retryMax: c.retryMax,
		jobs:     jobs,
		notify:   notify,
		cmd:      c.cmd,
		args:     c.args,
	}, nil
}

func (c *config) newJobs() (jobs, error) {
	if c.redis != "" {
		return loadRedisJobs(c.redis)
	}
	return newJobs(c.msgcache)
}

func loadRedisJobs(path string) (jobs, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var opt redisJobsOptions
	if err := json.Unmarshal(b, &opt); err != nil {
		return nil, err
	}
	return newRedisJobs(opt)
}
