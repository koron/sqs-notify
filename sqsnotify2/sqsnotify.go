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
	Profile   string
	Region    string
	QueueName string
	Workers   int
	CmdName   string
	CmdArgs   []string

	l sync.Mutex

	results []*result
}

func (sn *SQSNotify) Run() error {
	svc, err := sn.newSQS()
	if err != nil {
		return err
	}
	return sn.run(svc)
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
	return sqs.New(s, cfg), nil
}

func (sn *SQSNotify) run(api sqsiface.SQSAPI) error {
	qu, err := getQueueUrl(api, sn.QueueName)
	if err != nil {
		return err
	}
	for {
		// receive messages.
		msgs, err := sn.receive(api, qu, maxMsg)
		if err != nil {
			return err
		}

		// run as commands
		ctx := context.Background()
		sem := sn.newWeighted()
		var wg sync.WaitGroup
		for _, m := range msgs {
			wg.Add(1)
			go func(m *sqs.Message) {
				defer wg.Done()
				err := sem.Acquire(ctx, 1)
				if err != nil {
					sn.addResult(&result{msg: m, err: err})
					return
				}
				defer sem.Release(1)
				err = sn.execCmd(ctx, m)
				if err != nil {
					sn.addResult(&result{msg: m, err: err})
					return
				}
				sn.addResult(&result{msg: m})
			}(m)
		}
		wg.Wait()

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
		err = sn.delete(api, qu, entries)
		if err != nil {
			return err
		}
		sn.clearResults()
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

func (sn *SQSNotify) receive(api sqsiface.SQSAPI, queueUrl *string, max int64) ([]*sqs.Message, error) {
	// TODO: retry n times if failed.
	msgs, err := receiveMessages(api, queueUrl, maxMsg)
	if err != nil {
		return nil, err
	}
	return msgs, nil
}

func (sn *SQSNotify) delete(api sqsiface.SQSAPI, queueUrl *string, entries []*sqs.DeleteMessageBatchRequestEntry) error {
	// TODO: retry n times if failed with rebuild entries which failed.
	// TODO: ignore failed entires because of not exists already.
	err := deleteMessages(api, queueUrl, entries)
	if err != nil {
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
	msg *sqs.Message
	err error
}

func (r *result) shouldRemove() bool {
	// FIXME:
	return r.err == nil
}
