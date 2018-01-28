package sqsnotify2

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
)

func getQueueUrl(api sqsiface.SQSAPI, queueName string) (*string, error) {
	out, err := api.GetQueueUrl(&sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	})
	if err != nil {
		return nil, err
	}
	return out.QueueUrl, nil
}

func receiveMessages(api sqsiface.SQSAPI, queueUrl *string, max int64) ([]*sqs.Message, error) {
	out, err := api.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueUrl:            queueUrl,
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

func deleteMessages(api sqsiface.SQSAPI, queueUrl *string, entries []*sqs.DeleteMessageBatchRequestEntry) error {
	out, err := api.DeleteMessageBatch(&sqs.DeleteMessageBatchInput{
		QueueUrl: queueUrl,
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
