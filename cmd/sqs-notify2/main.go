package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/aws/aws-sdk-go/aws/awserr"
	valid "github.com/koron/go-valid"
	"github.com/koron/hupwriter"
	"github.com/koron/sqs-notify/sqsnotify2"
)

const (
	rpSucceed         = "succeed"
	rpIgnoreFailure   = "ignore_failure"
	rpBeforeExecution = "before_execution"
)

func toRP(s string) sqsnotify2.RemovePolicy {
	switch s {
	default:
		fallthrough
	case rpSucceed:
		return sqsnotify2.Succeed
	case rpIgnoreFailure:
		return sqsnotify2.IgnoreFailure
	case rpBeforeExecution:
		return sqsnotify2.BeforeExecution
	}
}

func main2() error {
	var (
		cfg     = sqsnotify2.NewConfig()
		version bool
		logfile string
		pidfile string

		waitTimeSec int64
		removePolicy string
	)

	flag.StringVar(&cfg.Profile, "profile", "", "AWS profile name")
	flag.StringVar(&cfg.Region, "region", "us-east-1", "AWS region")
	flag.Var(valid.String(&cfg.QueueName, "").MustSet(), "queue", "SQS queue name")
	flag.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, "max retries for AWS")
	flag.Int64Var(&waitTimeSec, "wait-time-seconds", -1, `wait time in seconds for next polling. (default -1, disabled, use queue default)`)

	flag.StringVar(&cfg.CacheName, "cache", cfg.CacheName,
		`cache name or connection URL
 * memory://?capacity=1000
 * redis://[{USER}:{PASS}@]{HOST}/[{DBNUM}]?[{OPTIONS}]

   DBNUM: redis DB number (default 0)
   OPTIONS:
	* lifetime : lifetime of cachetime (ex. "10s", "2m", "3h")
	* prefix   : prefix of keys

   Example to connect the redis on localhost: "redis://:6379"`)

	flag.IntVar(&cfg.Workers, "workers", cfg.Workers, "num of workers")
	flag.DurationVar(&cfg.Timeout, "timeout", 0, "timeout for command execution (default 0 - no timeout)")
	flag.Var(valid.String(&removePolicy, rpSucceed).
		OneOf(rpSucceed, rpIgnoreFailure, rpBeforeExecution), "remove-policy",
		`policy to remove messages from SQS
 * succeed          : after execution, succeeded (default)
 * ignore_failure   : after execution, ignore its result
 * before_execution : before execution`)
	flag.BoolVar(&version, "version", false, "show version")
	flag.StringVar(&logfile, "logfile", "", "log file path")
	flag.StringVar(&pidfile, "pidfile", "", "PID file path (require -logfile)")
	if err := valid.Parse(flag.CommandLine, os.Args[1:]); err != nil {
		return err
	}

	if version {
		fmt.Println("sqs-notify2 version:", sqsnotify2.Version)
		os.Exit(1)
	}

	if flag.NArg() < 1 {
		return errors.New("need a notification command")
	}
	args := flag.Args()
	cfg.RemovePolicy = toRP(removePolicy)
	cfg.CmdName = args[0]
	cfg.CmdArgs = args[1:]
	if waitTimeSec >= 0 {
		cfg.WaitTime = &waitTimeSec
	}

	// Setup logger.
	// FIXME: test logging features.
	if pidfile != "" && logfile == "" {
		return errors.New("pidfile option requires logfile option")
	}
	if logfile != "" {
		if logfile == "-" {
			cfg.Logger = log.New(os.Stdout, "", log.LstdFlags)
		} else {
			cfg.Logger = log.New(hupwriter.New(logfile, pidfile), "", 0)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	go func() {
		for {
			s := <-sig
			if s == os.Interrupt {
				cancel()
				signal.Stop(sig)
				close(sig)
				return
			}
		}
	}()
	signal.Notify(sig, os.Interrupt)

	cache, err := sqsnotify2.NewCache(ctx, cfg.CacheName)
	if err != nil {
		return err
	}
	defer cache.Close()

	err = sqsnotify2.New(cfg).Run(ctx, cache)
	if err != nil {
		if isCancel(err) {
			return nil
		}
		return err
	}

	return nil
}

func isCancel(err error) bool {
	if err2, ok := err.(awserr.Error); ok {
		err = err2.OrigErr()
	}
	if err == context.Canceled {
		return true
	}
	return false
}

func main() {
	err := main2()
	if err != nil {
		log.Fatal(err)
	}
}
