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

func usage() {
	fmt.Printf(`Usage: %s [OPTIONS] {queue name} {command}

OPTIONS:
  -region {region} :    name of region

Environment variables:
  AWS_ACCESS_KEY_ID
  AWS_SECRET_ACCESS_KEY
`, os.Args[0])
	os.Exit(1)
}

func runCmd(cmd *exec.Cmd, msgbody string) (err error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return
	}
	err = cmd.Start()
	if err != nil {
		return
	}
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	go func() {
		stdin.Write([]byte(msgbody))
		stdin.Close()
	}()
	return cmd.Wait()
}

func parseArgs() (*sqsnotify.SQSNotify, error) {
	var regionName string
	flag.StringVar(&regionName, "region", "us-east-1", "AWS Region for queue")
	flag.Parse()

	// Parse arguments.
	args := flag.Args()
	if len(args) < 2 {
		usage()
	}
	queueName := args[0]

	// Determine a region.
	region, ok := aws.Regions[regionName]
	if !ok {
		return nil, errors.New("unknown region:" + regionName)
	}

	// Retrieve an AWS auth.
	auth, err := aws.EnvAuth()
	if err != nil {
		return nil, err
	}

	return sqsnotify.New(auth, region, queueName), nil
}

func run(n *sqsnotify.SQSNotify) (err error) {
	// Open a queue.
	err = n.Open()
	if err != nil {
		return
	}

	// Listen queue.
	c, err := n.Listen()
	if err != nil {
		return
	}

	// Receive *sqsnotify.SQSMessage via channel.
	for _ = range c {
		// TODO:
	}

	return
}

func main() {
	n, err := parseArgs()
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}
	err = run(n)
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}
}
