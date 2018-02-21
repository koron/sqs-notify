package sqsnotify2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
)

func getQueueURL(api sqsiface.SQSAPI, queueName string) (*string, error) {
	out, err := api.GetQueueUrl(&sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	})
	if err != nil {
		return nil, err
	}
	return out.QueueUrl, nil
}

func receiveMessages(ctx context.Context, api sqsiface.SQSAPI, queueURL *string, max int64) ([]*sqs.Message, error) {
	out, err := api.ReceiveMessageWithContext(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            queueURL,
		MaxNumberOfMessages: &max,
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
