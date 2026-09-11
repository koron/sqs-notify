// Package sqsnotify provides sqs-notify2 core feature.
package sqsnotify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/smithy-go"
	"github.com/koron/sqs-notify/v2/internal/cache"
	"github.com/koron/sqs-notify/v2/internal/stage"
	"golang.org/x/sync/semaphore"
)

const MaxMsg = 10

var discardLog = log.New(io.Discard, "", 0)

// SQSNotify provides SQS consumer and job manager.
type SQSNotify struct {
	Config

	l       sync.Mutex
	results []*result
	cache   cache.Cache

	sqsClient *sqs.Client
	queueURL  string

	queueVisibilityTimeout time.Duration
}

// New creates a SQSNotify object with configuration.
func New(cfg *Config) *SQSNotify {
	if cfg == nil {
		cfg = NewConfig()
	}
	return &SQSNotify{
		Config:                 *cfg,
		queueVisibilityTimeout: 30 * time.Second,
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
func (sn *SQSNotify) Run(ctx context.Context, cache cache.Cache) error {
	svc, err := sn.newSQS(ctx)
	if err != nil {
		return err
	}
	sn.cache = cache
	sn.sqsClient = svc

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

func isQueueDoesNotExist(err error) bool {
	var qne *types.QueueDoesNotExist
	if errors.As(err, &qne) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "AWS.SimpleQueueService.NonExistentQueue" || code == "QueueDoesNotExist"
	}
	return false
}

func (sn *SQSNotify) ensureQueue(ctx context.Context, client *sqs.Client) (string, error) {
	rGet, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(sn.QueueName),
	})
	if err == nil {
		return *rGet.QueueUrl, nil
	}
	if !sn.CreateQueue || !isQueueDoesNotExist(err) {
		return "", err
	}

	rCreate, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(sn.QueueName),
	})
	if err != nil {
		return "", err
	}
	return *rCreate.QueueUrl, nil
}

func (sn *SQSNotify) run(ctx context.Context, client *sqs.Client) error {
	qu, err := sn.ensureQueue(ctx, client)
	if err != nil {
		return err
	}
	sn.queueURL = qu

	// Get visibility timeout.
	if sn.AutoExtend {
		out, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
			QueueUrl: &qu,
			AttributeNames: []types.QueueAttributeName{
				types.QueueAttributeNameVisibilityTimeout,
			},
		})
		if err != nil {
			return err
		}
		if timeout, ok := out.Attributes[string(types.QueueAttributeNameVisibilityTimeout)]; ok {
			sec, err := strconv.Atoi(timeout)
			if err != nil {
				return err
			}
			sn.queueVisibilityTimeout = time.Duration(sec) * time.Second
		}
	}

	var round = 0
	for {
		// receive messages.
		msgs, err := sn.receiveQ(ctx, client, &qu, MaxMsg)
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
			err := sn.deleteQ(ctx, client, &qu, entries)
			if err != nil {
				return err
			}
		}

		// Start the loop for auto extension of visibility timeout
		var extendCancel context.CancelFunc = func() {}
		if sn.AutoExtend && sn.RemovePolicy != BeforeExecution {
			entries := make([]types.ChangeMessageVisibilityBatchRequestEntry, len(msgs))
			for i, m := range msgs {
				entries[i] = types.ChangeMessageVisibilityBatchRequestEntry{
					Id:            m.MessageId,
					ReceiptHandle: m.ReceiptHandle,
				}
			}
			extendCtx, cancel := context.WithCancel(ctx)
			extendCancel = cancel
			go sn.autoExtendQLoop(extendCtx, entries)
		}

		// run as commands
		sem := semaphore.NewWeighted(int64(min(max(1, sn.Workers), MaxMsg)))
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
		extendCancel()

		// delete messages
		err = sn.deleteQ(ctx, client, &qu, sn.deleteEntries())
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

func (sn *SQSNotify) receiveQ(ctx context.Context, client *sqs.Client, queueURL *string, max int32) ([]types.Message, error) {
	input := &sqs.ReceiveMessageInput{
		QueueUrl:            queueURL,
		MaxNumberOfMessages: max,
	}
	if sn.WaitTime != nil {
		input.WaitTimeSeconds = int32(*sn.WaitTime)
	}
	out, err := client.ReceiveMessage(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Messages, nil
}

func (sn *SQSNotify) autoExtendQLoop(ctx context.Context, entries []types.ChangeMessageVisibilityBatchRequestEntry) {
	currTimeout := sn.queueVisibilityTimeout
	for {
		wait := max(currTimeout*4/5, currTimeout-15*time.Second)
		if wait < time.Second {
			wait = time.Second
		}
		// Wait for the specified time to elapse or for the context to be stopped.
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		// Extend visibility timeout of the message.
		next := min(time.Duration(float64(currTimeout)*sn.AutoExtendFactor), sn.AutoExtendMax, 12*time.Hour)
		nextSec := int32(next.Seconds())
		for i := range entries {
			entries[i].VisibilityTimeout = nextSec
		}
		_, err := sn.sqsClient.ChangeMessageVisibilityBatch(ctx, &sqs.ChangeMessageVisibilityBatchInput{
			Entries:  entries,
			QueueUrl: &sn.queueURL,
		})
		if err != nil {
			sn.log().Printf("failed to extend message visibility timeout: err=%s", err)
			return
		}
		currTimeout = next
	}
}

type DeleteFailure struct {
	Failed []types.BatchResultErrorEntry
}

func (df *DeleteFailure) Error() string {
	return fmt.Sprintf("failed to delete %d messages", len(df.Failed))
}

func (sn *SQSNotify) deleteQ(ctx context.Context, client *sqs.Client, queueURL *string, entries []types.DeleteMessageBatchRequestEntry) error {
	if len(entries) == 0 {
		return nil
	}

	out, err := client.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{
		QueueUrl: queueURL,
		Entries:  entries,
	})
	if err != nil {
		return err
	}
	if len(out.Failed) > 0 {
		// TODO: retry or skip failed entries.
		// 1. "not exists" be skipped (ignored)
		// 2. others are retried or logged
		return &DeleteFailure{Failed: out.Failed}
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
