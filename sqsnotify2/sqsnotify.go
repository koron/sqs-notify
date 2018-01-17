package sqsnotify2

import (
	"log"

	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
)

// SQSNotify provides SQS consumer and job manager.
type SQSNotify struct {
	QueueName string
}

func (sn *SQSNotify) Run() error {
	svc, err := sn.open()
	if err != nil {
		return err
	}
	return sn.run(svc)
}

func (sn *SQSNotify) open() (*sqs.SQS, error) {
	// TODO:
	return nil, nil
}

func (sn *SQSNotify) run(api sqsiface.SQSAPI) error {
	qu, err := getQueueUrl(api, sn.QueueName)
	if err != nil {
		return err
	}
	for {
		msgs, err := receiveMessages(api, qu, 10)
		err = sn.receiveError(err)
		if err != nil {
			return err
		}
		// TODO:
		_ = msgs
	}
}

func (sn *SQSNotify) receiveError(err error) error {
	// TODO:
	return err
}

func (sn *SQSNotify) logErr(err error) {
	// TODO:
	log.Println(err)
}
