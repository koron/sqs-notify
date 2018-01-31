package sqsnotify2

import (
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
	"golang.org/x/sync/semaphore"
)

const maxMsg = 10

// SQSNotify provides SQS consumer and job manager.
type SQSNotify struct {
	Config
	l       sync.Mutex
	results []*result
}

// New creates a SQSNotify object with configuration.
func New(cfg *Config) *SQSNotify {
	if cfg == nil {
		cfg = NewConfig()
	}
	return &SQSNotify{
		Config: *cfg,
	}
}

// Run runs SQS notification service.
// ctx is not supported yet.
func (sn *SQSNotify) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	svc, err := sn.newSQS()
	if err != nil {
		return err
	}
	return sn.run(ctx, svc)
}

func (sn *SQSNotify) newSQS() (*sqs.SQS, error) {
	s, err := session.NewSessionWithOptions(session.Options{
		Profile: sn.Profile,
	})
	if err != nil {
		return nil, err
	}
	cfg := aws.NewConfig()
	if sn.Region != "" {
		cfg.WithRegion(sn.Region)
	}
	if sn.MaxRetries > 0 {
		cfg.WithMaxRetries(sn.MaxRetries)
	}
	return sqs.New(s, cfg), nil
}

func (sn *SQSNotify) run(ctx context.Context, api sqsiface.SQSAPI) error {
	qu, err := getQueueUrl(api, sn.QueueName)
	if err != nil {
		return err
	}
	var round = 0
	for {
		// receive messages.
		msgs, err := sn.receiveQ(api, qu, maxMsg)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		// run as commands
		sem := sn.newWeighted()
		var wg sync.WaitGroup
		for i, m := range msgs {
			wg.Add(1)
			go func(r, n int, m *sqs.Message) {
				defer wg.Done()
				err := sem.Acquire(ctx, 1)
				if err != nil {
					sn.addResult(&result{round: r, index: n, msg: m, err: err})
					return
				}
				defer sem.Release(1)
				err = sn.execCmd(ctx, m)
				if err != nil {
					sn.addResult(&result{round: r, index: n, msg: m, err: err})
					return
				}
				sn.addResult(&result{round: r, index: n, msg: m})
			}(round, i, m)
		}
		wg.Wait()
		if err := ctx.Err(); err != nil {
			return err
		}

		// delete messages
		var entries []*sqs.DeleteMessageBatchRequestEntry
		for _, r := range sn.results {
			if !r.shouldRemove() {
				continue
			}
			entries = append(entries, &sqs.DeleteMessageBatchRequestEntry{
				Id:            r.msg.MessageId,
				ReceiptHandle: r.msg.ReceiptHandle,
			})
		}
		err = sn.deleteQ(api, qu, entries)
		if err != nil {
			return err
		}
		sn.clearResults()
		if err := ctx.Err(); err != nil {
			return err
		}
		round++
	}
}

// execCmd executes a command for a message, and returns its exit code.
func (sn *SQSNotify) execCmd(ctx context.Context, m *sqs.Message) error {
	cmd := exec.CommandContext(ctx, sn.CmdName, sn.CmdArgs...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go io.Copy(os.Stdout, stdout)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	go io.Copy(os.Stderr, stderr)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	go func() {
		defer stdin.Close()
		_, err := io.WriteString(stdin, *m.Body)
		if err != nil {
			sn.handleCopyMessageFailure(err, m)
		}
	}()

	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func (sn *SQSNotify) receiveQ(api sqsiface.SQSAPI, queueUrl *string, max int64) ([]*sqs.Message, error) {
	msgs, err := receiveMessages(api, queueUrl, maxMsg)
	if err != nil {
		return nil, err
	}
	return msgs, nil
}

func (sn *SQSNotify) deleteQ(api sqsiface.SQSAPI, queueUrl *string, entries []*sqs.DeleteMessageBatchRequestEntry) error {
	err := deleteMessages(api, queueUrl, entries)
	if err != nil {
		if f, ok := err.(*deleteFailure); ok {
			// TODO: retry or skip failed entries.
			// 1. "not exists" be skipped (ignored)
			// 2. others are retried or logged
			_ = f
		}
		return err
	}
	return nil
}

func (sn *SQSNotify) handleCopyMessageFailure(err error, m *sqs.Message) {
	// TODO: show more details
	log.Printf("failed to copy message: %s", err)
}

func (sn *SQSNotify) newWeighted() *semaphore.Weighted {
	n := sn.Workers
	if n < 0 || n > maxMsg {
		n = 4
	}
	return semaphore.NewWeighted(int64(n))
}

func (sn *SQSNotify) clearResults() {
	sn.l.Lock()
	sn.results = sn.results[:0]
	sn.l.Unlock()
}

func (sn *SQSNotify) addResult(r *result) {
	sn.l.Lock()
	sn.results = append(sn.results, r)
	sn.l.Unlock()
}

type result struct {
	round int
	index int
	msg   *sqs.Message
	err   error
}

func (r *result) shouldRemove() bool {
	// FIXME:
	return r.err == nil
}