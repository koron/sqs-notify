package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/sqs"
	"github.com/koron/sqs-notify/awsutil"
)

const maxSend = 10

var (
	region = flag.String("r", "us-east-1", "AWS region for SQS")
	queue  = flag.String("q", "", "queue name to send")
	num    = flag.Int("n", 1, "number of message to send")
	prefix = flag.String("p", "", "prefix for messages")
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	flag.Parse()
	if *queue == "" {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "need to specify queue name")
		os.Exit(1)
	}
	err := send(*region, *queue, *prefix, *num)
	if err != nil {
		log.Printf("fail to send: %s", err)
	}
}

func openQueue(rname, qname string) (*sqs.Queue, error) {
	auth, err := awsutil.GetAuth("sqs-send")
	if err != nil {
		return nil, err
	}
	region, ok := aws.Regions[rname]
	if !ok {
		return nil, errors.New("unknown region: " + rname)
	}
	queue, err := sqs.New(auth, region).GetQueue(qname)
	if err != nil {
		return nil, err
	}
	return queue, nil
}

func send(rname, qname, prefix string, num int) error {
	queue, err := openQueue(rname, qname)
	if err != nil {
		return err
	}
	msg := make([]string, 0, maxSend)
	ts := time.Now().Format(time.RFC3339)
	for i := 0; i < num; {
		msg = msg[:0]
		n := min(num-i, maxSend)
		for j := i; j < i+n; j++ {
			msg = append(msg, fmt.Sprintf("%s%s #%d", prefix, ts, j+1))
		}
		_, err := queue.SendMessageBatchString(msg)
		if err != nil {
			return err
		}
		for _, s := range msg {
			log.Printf("sent %q", s)
		}
		i += n
	}
	return nil
}
