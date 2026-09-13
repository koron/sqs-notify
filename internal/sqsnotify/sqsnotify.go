// Package sqsnotify provides sqs-notify core feature.
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
	"sync/atomic"
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

	cache          cache.Cache
	sqsClient      *sqs.Client
	queueURL       string
	initialTimeout time.Duration

	extendedTimeout atomic.Bool

	resultMu sync.Mutex
	results  []*result
}

// New creates a SQSNotify object with configuration.
func New(cfg *Config) *SQSNotify {
	if cfg == nil {
		cfg = NewConfig()
	}
	return &SQSNotify{
		Config:         *cfg,
		initialTimeout: 30 * time.Second,
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

// Run executes the SQS message monitoring/notification loop. Non-reentrant.
func (sn *SQSNotify) Run(ctx context.Context, cache cache.Cache) error {
	sqsClient, err := sn.newSQS(ctx)
	if err != nil {
		return err
	}

	queueURL, err := sn.ensureQueue(ctx, sqsClient)
	if err != nil {
		return err
	}

	if sn.AutoExtend {
		timeout, err := sn.getVisibilityTimeout(ctx, sqsClient, queueURL)
		if err != nil {
			return err
		}
		sn.initialTimeout = timeout
	}

	sn.cache = cache
	sn.sqsClient = sqsClient
	sn.queueURL = queueURL

	return sn.run(ctx)
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

func (sn *SQSNotify) getVisibilityTimeout(ctx context.Context, client *sqs.Client, queueURL string) (time.Duration, error) {
	out, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: &queueURL,
		AttributeNames: []types.QueueAttributeName{
			types.QueueAttributeNameVisibilityTimeout,
		},
	})
	if err != nil {
		return 0, err
	}
	timeout, ok := out.Attributes[string(types.QueueAttributeNameVisibilityTimeout)]
	if !ok {
		return 30 * time.Second, nil
	}
	sec, err := strconv.Atoi(timeout)
	if err != nil {
		return 0, err
	}
	return time.Duration(sec) * time.Second, nil
}

// createGracefulContext creates a new context.Context that remains active for
// an additional grace period after the parent context is cancelled.
func createGracefulContext(parent context.Context, gracePeriod time.Duration) (context.Context, context.CancelFunc) {
	baseCtx := context.WithoutCancel(parent)
	graceCtx, cancel := context.WithCancel(baseCtx)
	stop := context.AfterFunc(parent, func() {
		timer := time.AfterFunc(gracePeriod, func() {
			cancel()
		})
		_ = timer
	})
	return graceCtx, func() {
		stop()
		cancel()
	}
}

func (sn *SQSNotify) run(ctx context.Context) error {
	commandCtx, cmdCancel := createGracefulContext(ctx, sn.GracePeriodCommand)
	defer cmdCancel()
	cleanupCtx, deleteCancel := createGracefulContext(ctx, sn.GracePeriodCleanup)
	defer deleteCancel()

	var round = 0
	for {
		sn.extendedTimeout.Store(false)
		// receive messages.
		msgs, err := sn.receiveQ(ctx, MaxMsg)
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
			err := sn.deleteQ(ctx, entries)
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
					if errors.Is(err, context.Canceled) {
						sn.cacheDelete(res, stage.Canceled)
					}
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

				err = sn.execCmd(commandCtx, &m)
				if err != nil {
					if errors.Is(commandCtx.Err(), context.Canceled) {
						sn.cacheDelete(res, stage.Canceled)
					}
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

		resetEntries, deleteEntries := sn.cleanupResults()
		if len(resetEntries) > 0 && sn.extendedTimeout.Load() {
			err := sn.changeVisibilityQ(cleanupCtx, 0, resetEntries)
			if err != nil {
				sn.log().Printf("failed to reset message visiblity: %s", err)
			}
		}
		if len(deleteEntries) > 0 {
			err := sn.deleteQ(cleanupCtx, deleteEntries)
			if err != nil {
				sn.log().Printf("failed to delete messages: %s", err)
			}
		}

		sn.clearResults()
		round++

		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (sn *SQSNotify) cleanupResults() ([]types.ChangeMessageVisibilityBatchRequestEntry, []types.DeleteMessageBatchRequestEntry) {
	sn.resultMu.Lock()
	defer sn.resultMu.Unlock()
	var (
		resetEntries  []types.ChangeMessageVisibilityBatchRequestEntry
		deleteEntries []types.DeleteMessageBatchRequestEntry
	)
	for _, r := range sn.results {
		if r.stg == stage.Canceled {
			resetEntries = append(resetEntries, types.ChangeMessageVisibilityBatchRequestEntry{
				Id:            r.msg.MessageId,
				ReceiptHandle: r.msg.ReceiptHandle,
			})
			// For the result of `stage.Canceled`, `shouldRemoveAfter` always
			// returns `false`, so there is no need to determine whether to
			// delete it.
			continue
		}
		if sn.shouldRemoveAfter(r) {
			deleteEntries = append(deleteEntries, types.DeleteMessageBatchRequestEntry{
				Id:            r.msg.MessageId,
				ReceiptHandle: r.msg.ReceiptHandle,
			})
		}
	}
	sn.results = sn.results[:0]
	return resetEntries, deleteEntries
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

func (sn *SQSNotify) cacheDelete(r *result, stg stage.Stage) error {
	r.stg = stg
	if r.msg.MessageId == nil {
		return nil
	}
	return sn.cache.Delete(*r.msg.MessageId)
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

func (sn *SQSNotify) receiveQ(ctx context.Context, max int32) ([]types.Message, error) {
	input := &sqs.ReceiveMessageInput{
		QueueUrl:            &sn.queueURL,
		MaxNumberOfMessages: max,
	}
	if sn.WaitTime != nil {
		input.WaitTimeSeconds = int32(*sn.WaitTime)
	}
	out, err := sn.sqsClient.ReceiveMessage(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Messages, nil
}

func (sn *SQSNotify) autoExtendQLoop(ctx context.Context, entries []types.ChangeMessageVisibilityBatchRequestEntry) {
	currTimeout := sn.initialTimeout
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
		err := sn.changeVisibilityQ(ctx, int32(next.Seconds()), entries)
		if err != nil {
			sn.log().Printf("failed to extend message visibility timeout: err=%s", err)
			return
		}
		currTimeout = next
		sn.extendedTimeout.Store(true)
	}
}

func (sn *SQSNotify) changeVisibilityQ(ctx context.Context, sec int32, entries []types.ChangeMessageVisibilityBatchRequestEntry) error {
	if len(entries) == 0 {
		return nil
	}
	for i := range entries {
		entries[i].VisibilityTimeout = sec
	}
	_, err := sn.sqsClient.ChangeMessageVisibilityBatch(ctx, &sqs.ChangeMessageVisibilityBatchInput{
		Entries:  entries,
		QueueUrl: &sn.queueURL,
	})
	return err
}

type DeleteFailure struct {
	Failed []types.BatchResultErrorEntry
}

func (df *DeleteFailure) Error() string {
	return fmt.Sprintf("failed to delete %d messages", len(df.Failed))
}

func (sn *SQSNotify) deleteQ(ctx context.Context, entries []types.DeleteMessageBatchRequestEntry) error {
	if len(entries) == 0 {
		return nil
	}

	out, err := sn.sqsClient.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{
		QueueUrl: &sn.queueURL,
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
	sn.resultMu.Lock()
	sn.results = sn.results[:0]
	sn.resultMu.Unlock()
}

func (sn *SQSNotify) addResult(r *result) {
	sn.logResult(r)
	sn.resultMu.Lock()
	sn.results = append(sn.results, r)
	sn.resultMu.Unlock()
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
