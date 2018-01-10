package sqsnotify

import (
	"container/list"
	"fmt"
	"io/ioutil"
	"log"
	"sync"

	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/sqs"
)

// MessageCount specifies message amount to get at once.
var MessageCount = 1

// Logger provides log for sqsnotify.
var Logger = log.New(ioutil.Discard, "", log.LstdFlags)

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

	// for delete queue
	dql   sync.Mutex
	dqID  *list.List
	dqMsg map[string]sqs.Message

	// FailMax is limit for continuous errors.
	FailMax int

	failCnt int
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
	n.dqID = list.New()
	n.dqMsg = make(map[string]sqs.Message)
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

func (n *SQSNotify) addDeleteQueue(m *SQSMessage) {
	if m.IsEmpty() || m.deleted {
		return
	}
	id := m.Message.MessageId
	n.dql.Lock()
	defer n.dql.Unlock()
	if _, ok := n.dqMsg[id]; ok {
		// update ReceiptHandle (github#19)
		n.dqMsg[id] = m.Message
		return
	}
	n.dqMsg[id] = m.Message
	n.dqID.PushBack(id)
	m.deleted = true
}

// ReserveDelete reserves to delete message.
func (n *SQSNotify) ReserveDelete(m *SQSMessage) {
	n.addDeleteQueue(m)
	// flush to delete ASAP when the queue beyond max delete messages.
	if n.dqID.Len() >= maxDelete {
		_ = n.flushDeleteQueue()
	}
}

type deleteFault struct {
	ID   string
	Code string
}

func (n *SQSNotify) logDeleteMessageBatchError(resp *sqs.DeleteMessageBatchResponse, err error) {
	var faults []deleteFault
	for _, r := range resp.DeleteMessageBatchResult {
		if !r.SenderFault {
			continue
		}
		faults = append(faults, deleteFault{ID: r.Id, Code: r.Code})
	}
	if len(faults) > 0 {
		Logger.Printf("\tDELETE_FAULTS\t%+v", faults)
	}
	if err != nil {
		Logger.Printf("\tDELETE_ERROR\terror:%s", err)
	}
}

func (n *SQSNotify) flushDeleteQueue() error {
	err := n.flushDeleteQueue0()
	if err == nil {
		n.failCnt = 0
		return nil
	}
	if n.FailMax > 0 {
		n.failCnt++
		if n.failCnt >= n.failCnt {
			// TODO: better failure propagation. (github#19)
			panic(fmt.Sprintf("delete fails last %d times", n.failCnt))
		}
	}
	return err
}

func (n *SQSNotify) flushDeleteQueue0() error {
	n.dql.Lock()
	defer n.dql.Unlock()
	for n.dqID.Len() > 0 {
		msgs := make([]sqs.Message, 0, maxDelete)
		el := n.dqID.Front()
		for el != nil && len(msgs) < maxDelete {
			id := el.Value.(string)
			el = el.Next()
			msgs = append(msgs, n.dqMsg[id])
		}
		resp, err := n.queue.DeleteMessageBatch(msgs)
		n.logDeleteMessageBatchError(resp, err)
		if err != nil {
			return err
		}
		for _, m := range msgs {
			delete(n.dqMsg, m.MessageId)
			n.dqID.Remove(n.dqID.Front())
		}
	}
	return nil
}

// Name returns queue name.
func (n *SQSNotify) Name() string {
	return n.name
}

// Stop terminates listen loop.
func (n *SQSNotify) Stop() {
	n.running = false
	_ = n.flushDeleteQueue0()
}

// SQSMessage represent a SQS message.
type SQSMessage struct {
	Error   error
	Message sqs.Message

	deleted bool
	queue   *sqs.Queue
}

// IsEmpty checks MessageId is empty or not.
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
