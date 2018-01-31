package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"

	valid "github.com/koron/go-valid"
	"github.com/koron/sqs-notify/sqsnotify2"
)

func main2() error {
	cfg := sqsnotify2.NewConfig()

	flag.StringVar(&cfg.Profile, "profile", "", "AWS profile name")
	flag.StringVar(&cfg.Region, "region", "us-east-1", "AWS region")
	flag.Var(valid.String(&cfg.QueueName, "").MustSet(), "queue", "SQS queue name")
	flag.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, "max retries for AWS")
	flag.IntVar(&cfg.Workers, "workers", cfg.Workers, "num of workers")
	if err := valid.Parse(flag.CommandLine, os.Args[1:]); err != nil {
		return err
	}
	if flag.NArg() < 1 {
		return errors.New("need a notification command")
	}
	args := flag.Args()
	cfg.CmdName = args[0]
	cfg.CmdArgs = args[1:]

	sn := sqsnotify2.New(cfg)
	err := sn.Run(context.Background())
	if err != nil {
		return err
	}

	return nil
}

func main() {
	err := main2()
	if err != nil {
		log.Fatal(err)
	}
}
