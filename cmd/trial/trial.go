package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ss, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return err
	}

	c := aws.NewConfig()
	if s := os.Getenv("SQS_REGION"); s != "" {
		c.WithRegion(s)
	}
	if s := os.Getenv("SQS_ENDPOINT"); s != "" {
		c.WithEndpoint(s)
	}
	qs := sqs.New(ss, c)

	r, err := qs.ListQueues(&sqs.ListQueuesInput{})
	if err != nil {
		return err
	}
	for i, qu := range r.QueueUrls {
		fmt.Printf("#%d %s\n", i, *qu)
		r, err := qs.GetQueueAttributes(&sqs.GetQueueAttributesInput{
			AttributeNames: []*string{aws.String("VisibilityTimeout")},
			QueueUrl:       qu,
		})
		if err != nil {
			return err
		}
		if s, ok := r.Attributes["VisibilityTimeout"]; ok {
			fmt.Printf("  VisibilityTimeout=%s\n", *s)
		}
	}

	r2, err := qs.GetQueueUrl(&sqs.GetQueueUrlInput{QueueName: aws.String("kaoriya")})
	if err != nil {
		return err
	}
	qu := r2.QueueUrl

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		m := fmt.Sprintf("Hello SQS trial!: %s", time.Now())
		r, err := qs.SendMessageWithContext(ctx, &sqs.SendMessageInput{
			MessageBody: aws.String(m),
			QueueUrl:    qu,
		})
		if err != nil {
			log.Printf("failed SendMessage: %s", err)
			return
		}
		if r.MessageId != nil {
			log.Printf("send MessageId=%s", *r.MessageId)
		} else {
			log.Printf("WARN: no MessageId")
		}
	}()

	ch := make(chan *sqs.Message)

	go func() {
		defer wg.Done()
		var done bool
		var n int64 = 0
		for {
			r, err := qs.ReceiveMessageWithContext(ctx, &sqs.ReceiveMessageInput{
				QueueUrl: qu,
			})
			if err != nil {
				log.Printf("failed ReceiveMessage: %s", err)
				return
			}
			for i, m := range r.Messages {
				log.Printf("receive #%d-%d message: MessageId=%s", n, i, *m.MessageId)
			}
			if !done {
				for i, m := range r.Messages {
					log.Printf("transfer #%d-%d message: MessageId=%s", n, i, *m.MessageId)
					ch <- m
				}
				close(ch)
				done = true
			}
			if len(r.Messages) > 0 {
				n++
			}
		}
	}()

	go func() {
		defer cancel()
		defer wg.Done()
		m := <-ch
		log.Printf("extending MessageId=%s", *m.MessageId)
		time.Sleep(27 * time.Second)
		var to int64 = 30
		for i := 0; i < 6; i++ {
			to += 10
			_, err := qs.ChangeMessageVisibilityWithContext(ctx, &sqs.ChangeMessageVisibilityInput{
				QueueUrl:          qu,
				ReceiptHandle:     m.ReceiptHandle,
				VisibilityTimeout: aws.Int64(to),
			})
			if err != nil {
				log.Printf("fail ChangeMessageVisibility: %s", err)
				return
			}
			log.Printf("extended additional 10 seconds: MessageId=%s", *m.MessageId)
			time.Sleep(10 * time.Second)
		}
		_, err := qs.DeleteMessageWithContext(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      qu,
			ReceiptHandle: m.ReceiptHandle,
		})
		if err != nil {
			log.Printf("failed DeleteMessage: %s", err)
		}
	}()

	wg.Wait()

	return nil
}
