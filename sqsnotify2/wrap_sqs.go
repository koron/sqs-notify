package sqsnotify2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
)

func getQueueURL(api sqsiface.SQSAPI, queueName string, create bool) (*string, error) {
	rGet, err := api.GetQueueUrl(&sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	})
	if err == nil {
		return rGet.QueueUrl, nil
	}
	if !create || !isQueueDoesNotExist(err) {
		return nil, err
	}

	rCreate, err := api.CreateQueue(&sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
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

func receiveMessages(ctx context.Context, api sqsiface.SQSAPI, queueURL *string, max int64, waitTime *int64) ([]*sqs.Message, error) {
	out, err := api.ReceiveMessageWithContext(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            queueURL,
		MaxNumberOfMessages: &max,
		WaitTimeSeconds:     waitTime,
	})
	if err != nil {
		return nil, err
	}
	return out.Messages, nil
}

type deleteFailure struct {
	failed []*sqs.BatchResultErrorEntry
}

func (f *deleteFailure) Error() string {
	return fmt.Sprintf("failed to delete %d messages", len(f.failed))
}

func deleteMessages(ctx context.Context, api sqsiface.SQSAPI, queueURL *string, entries []*sqs.DeleteMessageBatchRequestEntry) error {
	out, err := api.DeleteMessageBatchWithContext(ctx, &sqs.DeleteMessageBatchInput{
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
