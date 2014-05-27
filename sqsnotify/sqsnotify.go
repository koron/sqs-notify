package sqsnotify

import (
	"github.com/Mistobaan/sqs"
	"launchpad.net/goamz/aws"
)

const messageCount = 10

type SQSNotify struct {
	auth   aws.Auth
	region aws.Region
	name   string

	queue *sqs.Queue
}

func New(auth aws.Auth, region aws.Region, name string) *SQSNotify {
	return &SQSNotify{auth, region, name, nil}
}

func (n *SQSNotify) Open() (err error) {
	awsSQS := sqs.New(n.auth, n.region)
	n.queue, err = awsSQS.GetQueue(n.name)
	return
}

func (n *SQSNotify) Listen() (chan *SQSMessage, error) {
	ch := make(chan *SQSMessage, messageCount)
	go func() {
		for {
			resp, err := n.queue.ReceiveMessage(messageCount)
			if err != nil {
				ch <- newErrorMessage(err)
				continue
			}
			for _, m := range resp.Messages {
				ch <- newMessage(&m, n.queue)
			}
		}
	}()
	return ch, nil
}

// Get queue name.
func (n *SQSNotify) Name() string {
	return n.name
}

type SQSMessage struct {
	Error   error
	Message *sqs.Message

	deleted bool
	queue   *sqs.Queue
}

func newErrorMessage(err error) *SQSMessage {
	return &SQSMessage{err, nil, true, nil}
}

func newMessage(m *sqs.Message, q *sqs.Queue) *SQSMessage {
	return &SQSMessage{nil, m, false, q}
}

func (m *SQSMessage) Body() *string {
	if m.Message == nil {
		return nil
	} else {
		return &m.Message.Body
	}
}

func (m *SQSMessage) Delete() (err error) {
	if m.deleted {
		return nil
	}
	_, err = m.queue.DeleteMessage(m.Message)
	return
}
