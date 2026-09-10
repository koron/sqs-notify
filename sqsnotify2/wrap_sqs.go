package sqsnotify2

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/smithy-go"
)

// SQSClient defines the interface for SQS operations used by SQSNotify.
type SQSClient interface {
	GetQueueUrl(ctx context.Context, params *sqs.GetQueueUrlInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error)
	CreateQueue(ctx context.Context, params *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error)
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessageBatch(ctx context.Context, params *sqs.DeleteMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error)
}

func getQueueURL(ctx context.Context, api SQSClient, queueName string, create bool) (*string, error) {
	rGet, err := api.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	})
	if err == nil {
		return rGet.QueueUrl, nil
	}
	if !create || !isQueueDoesNotExist(err) {
		return nil, err
	}

	rCreate, err := api.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
	})
	if err != nil {
		return nil, err
	}
	return rCreate.QueueUrl, nil
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

func receiveMessages(ctx context.Context, api SQSClient, queueURL *string, max int32, waitTime *int32) ([]types.Message, error) {
	input := &sqs.ReceiveMessageInput{
		QueueUrl:            queueURL,
		MaxNumberOfMessages: max,
	}
	if waitTime != nil {
		input.WaitTimeSeconds = *waitTime
	}
	out, err := api.ReceiveMessage(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Messages, nil
}

type deleteFailure struct {
	failed []types.BatchResultErrorEntry
}

func (f *deleteFailure) Error() string {
	return fmt.Sprintf("failed to delete %d messages", len(f.failed))
}

func deleteMessages(ctx context.Context, api SQSClient, queueURL *string, entries []types.DeleteMessageBatchRequestEntry) error {
	out, err := api.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{
		QueueUrl: queueURL,
		Entries:  entries,
	})
	if err != nil {
		return err
	}
	if len(out.Failed) > 0 {
		return &deleteFailure{failed: out.Failed}
	}
	return nil
}
