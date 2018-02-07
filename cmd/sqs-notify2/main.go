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

func main2() error {
	var (
		cfg     = sqsnotify2.NewConfig()
		version bool
		logfile string
		pidfile string
	)

	flag.StringVar(&cfg.Profile, "profile", "", "AWS profile name")
	flag.StringVar(&cfg.Region, "region", "us-east-1", "AWS region")
	flag.Var(valid.String(&cfg.QueueName, "").MustSet(), "queue", "SQS queue name")
	flag.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, "max retries for AWS")
	flag.IntVar(&cfg.Workers, "workers", cfg.Workers, "num of workers")
	flag.BoolVar(&version, "version", false, "show version")
	flag.StringVar(&logfile, "logfile", "", "log file path")
	flag.StringVar(&pidfile, "pidfile", "", "PID file path (require -logfile)")
	if err := valid.Parse(flag.CommandLine, os.Args[1:]); err != nil {
		return err
	}

	if version {
		fmt.Printf(sqsnotify2.Version)
		os.Exit(1)
	}

	if flag.NArg() < 1 {
		return errors.New("need a notification command")
	}
	args := flag.Args()
	cfg.CmdName = args[0]
	cfg.CmdArgs = args[1:]

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

	err := sqsnotify2.New(cfg).Run(ctx)
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
