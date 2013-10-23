package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/Mistobaan/sqs"
	"io"
	"launchpad.net/goamz/aws"
	"log"
	"os"
	"os/exec"
)

type SQSNotify struct {
	auth   aws.Auth
	region aws.Region
	name   string

	queue *sqs.Queue
}

func New(auth aws.Auth, region aws.Region, name string) *SQSNotify {
	return &SQSNotify{auth, region, name, nil}
}

func (n *SQSNotify) Open() (err error) {
	awsSQS := sqs.New(n.auth, n.region)
	n.queue, err = awsSQS.GetQueue(n.name)
	return
}

func (n *SQSNotify) Listen(handler func(*sqs.Message, func() error) error) (err error) {
	for {
		resp, err := n.queue.ReceiveMessage(10)
		if err != nil {
			return err
		}

		for _, m := range resp.Messages {
			d := n.deleter(&m)
			err = handler(&m, d)
			if err != nil {
				return err
			}
		}
	}
}

func (n *SQSNotify) deleter(m *sqs.Message) func() error {
	deleted := false
	return func() (err error) {
		if deleted {
			return
		}
		deleted = true
		_, err = n.queue.DeleteMessage(m)
		return
	}
}

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

func args2notify() (*SQSNotify, error) {
	var regionName string
	flag.StringVar(&regionName, "region", "us-east-1",
		"AWS Region for queue")
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

	return New(auth, region, queueName), nil
}

func run(n *SQSNotify) (err error) {
	// Open a queue.
	err = n.Open()
	if err != nil {
		return
	}

	// Listen the queue.
	err = n.Listen(func(m *sqs.Message, d func() error) (err error) {
		// TODO:
		return nil
	})
	return
}

func main() {
	n, err := args2notify()
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}
	err = run(n)
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}
}
