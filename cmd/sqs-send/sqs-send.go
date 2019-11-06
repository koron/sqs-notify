package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

const maxSend = 10

var (
	endpoint string
	region   string
	qname    string

	msgnum int
	prefix string
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	flag.StringVar(&endpoint, "endpoint", "", "endpoint of SQS")
	flag.StringVar(&region, "r", "us-east-1", "AWS region for SQS")
	flag.StringVar(&qname, "q", "", "queue name to send")
	flag.IntVar(&msgnum, "n", 1, "number of message to send")
	flag.StringVar(&prefix, "p", "", "prefix for messages")
	flag.Parse()
	if qname == "" {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "need to specify queue name")
		os.Exit(1)
	}
	err := sendMessages(context.Background())
	if err != nil {
		log.Printf("fail to send: %s", err)
	}
}

func ensureQueue(ctx context.Context, q *sqs.SQS, qn string) (*string, error) {
	rGet, err := q.GetQueueUrlWithContext(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(qn),
	})
	if err == nil {
		return rGet.QueueUrl, nil
	}
	if !isQueueDoesNotExist(err) {
		return nil, err
	}
	rCreate, err := q.CreateQueueWithContext(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(qn),
	})
	if err != nil {
		return nil, err
	}
	return rCreate.QueueUrl, nil
}

func isQueueDoesNotExist(err0 error) bool {
	err, ok := err0.(awserr.Error)
	if !ok {
		return false
	}
	return err.Code() == sqs.ErrCodeQueueDoesNotExist
}

func sendQueue(ctx context.Context, q *sqs.SQS, qurl *string, msgs []string) error {
	entries := make([]*sqs.SendMessageBatchRequestEntry, 0, len(msgs))
	for i, m := range msgs {
		entries = append(entries, &sqs.SendMessageBatchRequestEntry{
			Id:          aws.String(strconv.Itoa(i)),
			MessageBody: aws.String(m),
		})
	}
	_, err := q.SendMessageBatchWithContext(ctx, &sqs.SendMessageBatchInput{
		Entries:  entries,
		QueueUrl: qurl,
	})
	if err != nil {
		return err
	}
	return nil
}

func newSQS() (*sqs.SQS, error) {
	cfg := aws.NewConfig()
	if endpoint != "" {
		cfg.WithEndpoint(endpoint)
	}
	if region != "" {
		cfg.WithRegion(region)
	}
	ses, err := session.NewSession(cfg)
	if err != nil {
		return nil, err
	}
	return sqs.New(ses), nil
}

func sendMessages(ctx context.Context) error {
	q, err := newSQS()
	if err != nil {
		return err
	}

	qurl, err := ensureQueue(ctx, q, qname)
	if err != nil {
		return err
	}

	msgs := make([]string, 0, maxSend)
	for i := 0; i < msgnum; {
		msgs = msgs[:0]
		n := min(msgnum-i, maxSend)
		for j := i; j < i+n; j++ {
			msgs = append(msgs, fmt.Sprintf("%s%d", prefix, j+1))
		}
		err := sendQueue(ctx, q, qurl, msgs)
		if err != nil {
			return err
		}
		for _, m := range msgs {
			log.Printf("sent %q", m)
		}
		i += n
	}
	return nil
}
