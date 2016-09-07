package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/goamz/goamz/aws"
	"github.com/koron/hupwriter"
	"github.com/koron/sqs-notify/awsutil"
	"github.com/koron/sqs-notify/sqsnotify"
)

const (
	modeAtMostOnce = "at-most-once"
)

type config struct {
	daemon        bool
	region        string
	worker        int
	nowait        bool
	ignoreFailure bool
	messageCount  int
	digestID      bool
	retryMax      int
	msgcache      int
	redis         string
	logfile       string
	pidfile       string
	queue         string
	cmd           string
	args          []string

	l *log.Logger
}

func getConfig() (*config, error) {
	var (
		version       bool
		daemon        bool
		region        string
		worker        int
		nowait        bool
		ignoreFailure bool
		messageCount  int
		digestID      bool
		retryMax      int
		msgcache      int
		redis         string
		logfile       string
		pidfile       string
		mode          string
	)

	flag.BoolVar(&version, "version", false, "show version")
	flag.BoolVar(&daemon, "daemon", false, "run as a daemon")
	flag.StringVar(&region, "region", "us-east-1", "AWS Region for queue")
	flag.IntVar(&worker, "worker", 4, "Num of workers")
	flag.BoolVar(&nowait, "nowait", false, "Don't wait end of command")
	flag.BoolVar(&ignoreFailure, "ignorefailure", false, "Don't care command failures")
	flag.IntVar(&messageCount, "messagecount", 10, "retrieve multiple messages at once")
	flag.BoolVar(&digestID, "digest-id", false, "Use digest as message identifier")
	flag.IntVar(&retryMax, "retrymax", 4, "Num of retry count")
	flag.IntVar(&msgcache, "msgcache", 0, "Num of last messages in cache")
	flag.StringVar(&redis, "redis", "", "Use redis as messages cache")
	flag.StringVar(&logfile, "logfile", "", "Log file path")
	flag.StringVar(&pidfile, "pidfile", "", "PID file path (require -logfile)")
	flag.StringVar(&mode, "mode", "", "pre-defined set of options for specific usecases")
	flag.Usage = usage
	flag.Parse()

	if version {
		showVersion()
	}

	// Parse arguments.
	args := flag.Args()
	if len(args) < 2 {
		usage()
	}

	// Check consistencies of options
	if len(pidfile) > 0 && len(logfile) == 0 {
		return nil, errors.New("`-pidfile' requires `-logfile' option")
	}

	if messageCount <= 0 {
		return nil, errors.New("`-messagecount` should be > 0")
	} else if messageCount > 10 {
		return nil, errors.New("`-messagecount` should be <= 10")
	}

	// Apply modes.
	switch strings.ToLower(mode) {
	case modeAtMostOnce:
		if nowait {
			return nil, errors.New("`-nowait' conflicts with at-most-once")
		}
		if msgcache == 0 && redis == "" {
			return nil, errors.New("`-msgcache' or `-redis' is required for at-most-once")
		}
		nowait = false
		ignoreFailure = true
	}

	return &config{
		daemon:        daemon,
		region:        region,
		worker:        worker,
		nowait:        nowait,
		ignoreFailure: ignoreFailure,
		messageCount:  messageCount,
		digestID:      digestID,
		retryMax:      retryMax,
		msgcache:      msgcache,
		redis:         redis,
		logfile:       logfile,
		pidfile:       pidfile,
		queue:         args[0],
		cmd:           args[1],
		args:          args[2:],
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

	sqsnotify.Logger = c.logger()
	notify := sqsnotify.New(auth, region, c.queue)

	jobs, err := c.newJobs()
	if err != nil {
		return nil, err
	}

	return &app{
		logger:        c.logger(),
		auth:          auth,
		region:        region,
		worker:        c.worker,
		nowait:        c.nowait,
		ignoreFailure: c.ignoreFailure,
		messageCount:  c.messageCount,
		digestID:      c.digestID,
		retryMax:      c.retryMax,
		jobs:          jobs,
		notify:        notify,
		cmd:           c.cmd,
		args:          c.args,
	}, nil
}

func (c *config) logger() *log.Logger {
	if c.l != nil {
		return c.l
	}
	if len(c.logfile) > 0 {
		if c.logfile == "-" {
			c.l = log.New(os.Stdout, "", log.LstdFlags)
		} else {
			w := hupwriter.New(c.logfile, c.pidfile)
			c.l = log.New(w, "", log.LstdFlags)
		}
	}
	if c.l == nil {
		c.l = log.New(ioutil.Discard, "", 0)
	}
	return c.l
}

func (c *config) newJobs() (jobs, error) {
	if c.redis != "" {
		rj, err := loadRedisJobs(c.redis)
		if err != nil {
			return nil, err
		}
		rj.logger = c.logger()
		return rj, nil
	}
	return newJobs(c.msgcache)
}

func loadRedisJobs(path string) (*redisJobsManager, error) {
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
