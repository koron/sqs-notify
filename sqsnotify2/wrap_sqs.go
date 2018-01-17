package sqsnotify2

import (
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
