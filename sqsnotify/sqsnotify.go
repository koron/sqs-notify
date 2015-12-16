package sqsnotify

import (
	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/sqs"
)

const messageCount = 1

// SQSNotify provides SQS message stream.
type SQSNotify struct {
	auth   aws.Auth
	region aws.Region
	name   string

	queue *sqs.Queue

	running bool
}

// New creates and returns a SQSNotify instance.
func New(auth aws.Auth, region aws.Region, name string) *SQSNotify {
	return &SQSNotify{
		auth:    auth,
		region:  region,
		name:    name,
		queue:   nil,
		running: false,
	}
}

// Open prepare internal resources.
func (n *SQSNotify) Open() (err error) {
	awsSQS := sqs.New(n.auth, n.region)
	n.queue, err = awsSQS.GetQueue(n.name)
	return
}

// Listen starts the stream.
func (n *SQSNotify) Listen() (chan *SQSMessage, error) {
	ch := make(chan *SQSMessage, messageCount)
	go func() {
		n.running = true
	loop:
		for n.running {
			resp, err := n.queue.ReceiveMessage(messageCount)
			if err != nil {
				ch <- newErrorMessage(err)
				continue
			}
			for _, m := range resp.Messages {
				ch <- newMessage(&m, n.queue)
				if !n.running {
					break loop
				}
			}
		}
		close(ch)
	}()
	return ch, nil
}

// Name returns queue name.
func (n *SQSNotify) Name() string {
	return n.name
}

// Stop terminates listen loop.
func (n *SQSNotify) Stop() {
	n.running = false
}

// SQSMessage represent a SQS message.
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

// Body returns body of message.
func (m *SQSMessage) Body() *string {
	if m.Message == nil {
		return nil
	}
	return &m.Message.Body
}

// Delete requests to delete message to SQS.
func (m *SQSMessage) Delete() (err error) {
	if m.deleted {
		return nil
	}
	_, err = m.queue.DeleteMessage(m.Message)
	return
}
