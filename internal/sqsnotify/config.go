package sqsnotify

import (
	"log"
	"runtime"
	"time"
)

// RemovePolicy is a policy to remove SQS message.
type RemovePolicy int

const (
	// Succeed means "remove a message after notification succeeded"
	Succeed RemovePolicy = 0
	// IgnoreFailure means "remove a message after notification always"
	IgnoreFailure = 1
	// BeforeExecution means "remove a message before notification"
	BeforeExecution = 2
)

// Config configures sqsnotify service
type Config struct {
	Profile     string
	Region      string
	Endpoint    string
	QueueName   string
	CreateQueue bool
	MaxRetries  int
	WaitTime    *int64

	CacheName string

	Workers      int
	Timeout      time.Duration
	RemovePolicy RemovePolicy

	AutoExtend       bool
	AutoExtendFactor float64
	AutoExtendMax    time.Duration

	GracePeriodCommand time.Duration
	GracePeriodCleanup time.Duration

	CmdName string
	CmdArgs []string

	Logger *log.Logger
}

// NewConfig creates a new Config object.
func NewConfig() *Config {
	return &Config{
		Region:  "us-east-1",
		Workers: runtime.NumCPU(),

		AutoExtendFactor: 2.0,
		AutoExtendMax:    64 * time.Minute,

		GracePeriodCommand: 10 * time.Second,
		GracePeriodCleanup: 30 * time.Second,
	}
}
