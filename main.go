package main

import (
	"./sqsnotify"
	"errors"
	"flag"
	"fmt"
	"io"
	"launchpad.net/goamz/aws"
	"log"
	"os"
	"os/exec"
)

const progname = "sqs-notify"

type config struct {
	region string
	worker int
	queue string
	cmd string
	args []string
}

type app struct {
	auth aws.Auth
	region aws.Region
	worker int
	notify *sqsnotify.SQSNotify
	cmd string
	args []string
}

func usage() {
	fmt.Printf(`Usage: %s [OPTIONS] {queue name} {command and args...}

OPTIONS:
  -region {region} :    name of region (default: us-east-1)
  -worker {num} :       num of workers (default: 4)

Environment variables:
  AWS_ACCESS_KEY_ID
  AWS_SECRET_ACCESS_KEY
`, progname)
	os.Exit(1)
}

func getConfig() (*config, error) {
	var region string
	var worker int
	flag.StringVar(&region, "region", "us-east-1", "AWS Region for queue")
	flag.IntVar(&worker, "worker", 4, "Num of workers")
	flag.Parse()

	// Parse arguments.
	args := flag.Args()
	if len(args) < 2 {
		usage()
	}

	return &config{region, worker, args[0], args[1], args[2:]}, nil
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

	notify := sqsnotify.New(auth, region, c.queue)

	return &app{auth, region, c.worker, notify, c.cmd, c.args}, nil
}

func (a *app) run() (err error) {
	// Open a queue.
	err = a.notify.Open()
	if err != nil {
		return
	}

	// Listen queue.
	c, err := a.notify.Listen()
	if err != nil {
		return
	}

	w := NewWorkers(a.worker)

	// Receive *sqsnotify.SQSMessage via channel.
	for m := range c {
		if m.Error != nil {
			return m.Error
		}

		// Create and setup a exec.Cmd.
		cmd := exec.Command(a.cmd, a.args...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}

		w.Run(WorkerJob{cmd, func(r WorkerResult) {
			if r.ProcessState != nil && r.ProcessState.Success() {
				m.Delete()
				// FIXME: log it when failed to delete.
			}
		}})
		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)
		go func() {
			stdin.Write([]byte(*m.Body()))
			stdin.Close()
		}()
	}

	return
}

func main() {
	c, err := getConfig()
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}

	a, err := c.toApp()
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}

	err = a.run()
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}
}
