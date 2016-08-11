package sqsnotify

import (
	"sync"

	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/sqs"
)

// MessageCount specifies message amount to get at once.
var MessageCount = 1

const maxDelete = 10

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SQSNotify provides SQS message stream.
type SQSNotify struct {
	auth   aws.Auth
	region aws.Region
	name   string

	queue *sqs.Queue

	running bool

	deleteQueue []sqs.Message
	dql         sync.Mutex
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
	if err != nil {
		return err
	}
	n.deleteQueue = make([]sqs.Message, 0, maxDelete)
	return nil
}

// Listen starts the stream.
func (n *SQSNotify) Listen() (chan *SQSMessage, error) {
	ch := make(chan *SQSMessage, 1)
	go func() {
		n.running = true
	loop:
		for n.running {
			if err := n.flushDeleteQueue(); err != nil {
				ch <- newErrorMessage(err)
			}
			resp, err := n.queue.ReceiveMessage(MessageCount)
			if err != nil {
				ch <- newErrorMessage(err)
				continue
			}
			for _, m := range n.unique(resp.Messages) {
				ch <- newMessage(m, n.queue)
				if !n.running {
					break loop
				}
			}
		}
		close(ch)
	}()
	return ch, nil
}

func (n *SQSNotify) unique(list []sqs.Message) []sqs.Message {
	uniq := make([]sqs.Message, 0, len(list))
	index := make(map[string]int)
	for _, m := range list {
		k := m.MessageId
		n, ok := index[k]
		if ok {
			uniq[n] = m
			continue
		}
		index[k] = len(uniq)
		uniq = append(uniq, m)
	}
	return uniq
}

// ReserveDelete reserves to delete message.
func (n *SQSNotify) ReserveDelete(m *SQSMessage) {
	if m.IsEmpty() || m.deleted {
		return
	}
	n.dql.Lock()
	n.deleteQueue = append(n.deleteQueue, m.Message)
	m.deleted = true
	n.dql.Unlock()
	// flush to delete ASAP when 10 messages are reserved.
	if len(n.deleteQueue) >= maxDelete {
		n.flushDeleteQueue()
	}
}

func (n *SQSNotify) flushDeleteQueue() error {
	n.dql.Lock()
	defer n.dql.Unlock()
	if len(n.deleteQueue) == 0 {
		return nil
	}
	for q := n.deleteQueue; len(q) > 0; {
		l := min(len(q), maxDelete)
		_, err := n.queue.DeleteMessageBatch(q[0:l])
		if err != nil {
			// TODO: log messages which not be deleted.
			return err
		}
		q = q[l:]
	}
	n.deleteQueue = n.deleteQueue[:0]
	return nil
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
	Message sqs.Message

	deleted bool
	queue   *sqs.Queue
}

func (m SQSMessage) IsEmpty() bool {
	return m.Message.MessageId == ""
}

func newErrorMessage(err error) *SQSMessage {
	return &SQSMessage{
		Error:   err,
		deleted: true,
		queue:   nil,
	}
}

func newMessage(m sqs.Message, q *sqs.Queue) *SQSMessage {
	return &SQSMessage{
		Message: m,
		deleted: false,
		queue:   q,
	}
}

// ID returns MessageId of the message.
func (m *SQSMessage) ID() string {
	return m.Message.MessageId
}

// Body returns body of message.
func (m *SQSMessage) Body() *string {
	if m.IsEmpty() {
		return nil
	}
	return &m.Message.Body
}

// Delete requests to delete message to SQS.
func (m *SQSMessage) Delete() (err error) {
	if m.deleted {
		return nil
	}
	_, err = m.queue.DeleteMessage(&m.Message)
	if err == nil {
		m.deleted = true
	}
	return
}
