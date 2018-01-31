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
	"github.com/koron/sqs-notify/sqsnotify2"
)

func main2() error {
	var (
		cfg     = sqsnotify2.NewConfig()
		version bool
		daemon  bool
	)

	flag.StringVar(&cfg.Profile, "profile", "", "AWS profile name")
	flag.StringVar(&cfg.Region, "region", "us-east-1", "AWS region")
	flag.Var(valid.String(&cfg.QueueName, "").MustSet(), "queue", "SQS queue name")
	flag.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, "max retries for AWS")
	flag.IntVar(&cfg.Workers, "workers", cfg.Workers, "num of workers")
	flag.BoolVar(&version, "version", false, "show version")
	flag.BoolVar(&daemon, "daemon", false, "run as a daemon")
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

	if daemon {
		makeDaemon()
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
