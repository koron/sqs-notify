package sqsnotify2

import (
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/koron/sqs-notify/sqsnotify2/stage"
	"golang.org/x/sync/semaphore"
)

const maxMsg = 10

var discardLog = log.New(io.Discard, "", 0)

// SQSNotify provides SQS consumer and job manager.
type SQSNotify struct {
	Config

	l       sync.Mutex
	results []*result
	cache   Cache
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

func (sn *SQSNotify) log() *log.Logger {
	if sn.Config.Logger == nil {
		return discardLog
	}
	return sn.Config.Logger
}

func (sn *SQSNotify) logResult(r *result) {
	if r.err == nil {
		body := ""
		if r.msg.Body != nil {
			body = *r.msg.Body
		}
		sn.log().Printf("\tEXECUTED\tbody:%#v", body)
		return
	}
	sn.log().Printf("\tNOT_EXECUTED\tstage:%[2]s error:%[1]s", r.err, r.stg)
}

// Run runs SQS notification service.
func (sn *SQSNotify) Run(ctx context.Context, cache Cache) error {
	svc, err := sn.newSQS(ctx)
	if err != nil {
		return err
	}
	sn.cache = cache

	return sn.run(ctx, svc)
}

func (sn *SQSNotify) newSQS(ctx context.Context) (*sqs.Client, error) {
	var opts []func(*config.LoadOptions) error
	if sn.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(sn.Profile))
	}
	if sn.Region != "" {
		opts = append(opts, config.WithRegion(sn.Region))
	}
	if sn.MaxRetries > 0 {
		opts = append(opts, config.WithRetryMaxAttempts(sn.MaxRetries))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	var sqsOpts []func(*sqs.Options)
	if sn.Endpoint != "" {
		sqsOpts = append(sqsOpts, func(o *sqs.Options) {
			o.BaseEndpoint = aws.String(sn.Endpoint)
		})
	}
	return sqs.NewFromConfig(cfg, sqsOpts...), nil
}

func (sn *SQSNotify) run(ctx context.Context, api SQSClient) error {
	qu, err := getQueueURL(ctx, api, sn.QueueName, sn.CreateQueue)
	if err != nil {
		return err
	}
	var round = 0
	for {
		// receive messages.
		msgs, err := sn.receiveQ(ctx, api, qu, maxMsg)
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			//sn.log().Printf("round %d polling timed out, proceed next", round)
			round++
			continue
		}

		// remove messages first when RemovePolicy == BeforeExecution
		if sn.RemovePolicy == BeforeExecution {
			entries := make([]types.DeleteMessageBatchRequestEntry, 0, len(msgs))
			for _, m := range msgs {
				entries = append(entries, types.DeleteMessageBatchRequestEntry{
					Id:            m.MessageId,
					ReceiptHandle: m.ReceiptHandle,
				})
			}
			err := sn.deleteQ(ctx, api, qu, entries)
			if err != nil {
				return err
			}
		}

		// run as commands
		sem := sn.newWeighted()
		var wg sync.WaitGroup
		for i, m := range msgs {
			res := &result{round: round, index: i, msg: m}
			err := sn.cacheInsert(res, stage.Recv)
			if err != nil {
				sn.addResult(res.withErr(err))
				continue
			}
			wg.Add(1)
			go func(r, n int, m types.Message, res *result) {
				defer wg.Done()
				res.stg = stage.Lock
				err := sem.Acquire(ctx, 1)
				if err != nil {
					sn.addResult(res.withErr(err))
					return
				}
				defer sem.Release(1)
				res.stg = stage.Exec
				err = sn.cacheUpdate(res, stage.Exec)
				if err != nil {
					sn.addResult(res.withErr(err))
					return
				}
				err = sn.execCmd(ctx, &m)
				if err != nil {
					sn.addResult(res.withErr(err))
					return
				}
				res.stg = stage.Done
				err = sn.cacheUpdate(res, stage.Done)
				if err != nil {
					sn.addResult(res.withErr(err))
					return
				}
				sn.addResult(&result{round: r, index: n, msg: m})
			}(round, i, m, res)
		}
		wg.Wait()

		// delete messages
		err = sn.deleteQ(ctx, api, qu, sn.deleteEntries())
		if err != nil {
			return err
		}
		sn.clearResults()
		round++
	}
}

func (sn *SQSNotify) deleteEntries() []types.DeleteMessageBatchRequestEntry {
	var entries []types.DeleteMessageBatchRequestEntry
	for _, r := range sn.results {
		if !sn.shouldRemoveAfter(r) {
			continue
		}
		entries = append(entries, types.DeleteMessageBatchRequestEntry{
			Id:            r.msg.MessageId,
			ReceiptHandle: r.msg.ReceiptHandle,
		})
	}
	return entries
}

func (sn *SQSNotify) cacheInsert(r *result, stg stage.Stage) error {
	r.stg = stg
	if r.msg.MessageId == nil {
		return nil
	}
	err := sn.cache.Insert(*r.msg.MessageId, stg)
	if err != nil {
		return err
	}
	return nil
}

func (sn *SQSNotify) cacheUpdate(r *result, stg stage.Stage) error {
	r.stg = stg
	if r.msg.MessageId == nil {
		return nil
	}
	err := sn.cache.Update(*r.msg.MessageId, stg)
	if err != nil {
		// FIXME: consider errCacheNotFound
		return err
	}
	return nil
}

func (sn *SQSNotify) shouldRemoveAfter(r *result) bool {
	switch sn.RemovePolicy {
	default:
		fallthrough
	case Succeed:
		return r.err == nil
	case IgnoreFailure:
		if r.stg == stage.Exec {
			msgID := ""
			if r.msg.MessageId != nil {
				msgID = *r.msg.MessageId
			}
			sn.log().Printf("command failed but message is deleted: id=%s err=%s", msgID, r.err)
			return true
		}
		return r.err == nil
	case BeforeExecution:
		return false
	}
}

// execCmd executes a command for a message, and returns its exit code.
func (sn *SQSNotify) execCmd(ctx context.Context, m *types.Message) error {
	if sn.Timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sn.Timeout)
		defer cancel()
	}
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
		body := ""
		if m.Body != nil {
			body = *m.Body
		}
		_, err := io.WriteString(stdin, body)
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

func (sn *SQSNotify) receiveQ(ctx context.Context, api SQSClient, queueURL *string, max int32) ([]types.Message, error) {
	var wt *int32
	if sn.WaitTime != nil {
		v := int32(*sn.WaitTime)
		wt = &v
	}
	msgs, err := receiveMessages(ctx, api, queueURL, max, wt)
	if err != nil {
		return nil, err
	}
	return msgs, nil
}

func (sn *SQSNotify) deleteQ(ctx context.Context, api SQSClient, queueURL *string, entries []types.DeleteMessageBatchRequestEntry) error {
	if len(entries) == 0 {
		return nil
	}
	err := deleteMessages(ctx, api, queueURL, entries)
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

func (sn *SQSNotify) handleCopyMessageFailure(err error, m *types.Message) {
	msgID := ""
	if m.MessageId != nil {
		msgID = *m.MessageId
	}
	sn.log().Printf("failed to pass message body: id=%s err=%s", msgID, err)
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
	sn.logResult(r)
	sn.l.Lock()
	sn.results = append(sn.results, r)
	sn.l.Unlock()
}

type result struct {
	round int
	index int
	msg   types.Message
	stg   stage.Stage
	err   error
}

func (r *result) withErr(err error) *result {
	r.err = err
	return r
}
