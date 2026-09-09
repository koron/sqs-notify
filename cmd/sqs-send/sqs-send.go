package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
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

func ensureQueue(ctx context.Context, q *sqs.Client, qn string) (*string, error) {
	rGet, err := q.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(qn),
	})
	if err == nil {
		return rGet.QueueUrl, nil
	}
	if !isQueueDoesNotExist(err) {
		return nil, err
	}
	rCreate, err := q.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(qn),
	})
	if err != nil {
		return nil, err
	}
	return rCreate.QueueUrl, nil
}

func isQueueDoesNotExist(err error) bool {
	var qne *types.QueueDoesNotExist
	return errors.As(err, &qne)
}

func sendQueue(ctx context.Context, q *sqs.Client, qurl *string, msgs []string) error {
	entries := make([]types.SendMessageBatchRequestEntry, 0, len(msgs))
	for i, m := range msgs {
		entries = append(entries, types.SendMessageBatchRequestEntry{
			Id:          aws.String(strconv.Itoa(i)),
			MessageBody: aws.String(m),
		})
	}
	_, err := q.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{
		Entries:  entries,
		QueueUrl: qurl,
	})
	if err != nil {
		return err
	}
	return nil
}

func newSQS(ctx context.Context) (*sqs.Client, error) {
	var opts []func(*config.LoadOptions) error
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	var sqsOpts []func(*sqs.Options)
	if endpoint != "" {
		sqsOpts = append(sqsOpts, func(o *sqs.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	return sqs.NewFromConfig(cfg, sqsOpts...), nil
}

func sendMessages(ctx context.Context) error {
	q, err := newSQS(ctx)
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
