package sqsnotify2

import (
	"log"
	"runtime"
)

// RemovePolicy is a policy to remove SQS message.
type RemovePolicy int

const (
	// Succeed means "remove a message after notification succeeded"
	Succeed         RemovePolicy = 0
	// IgnoreFailure means "remove a message after notification always"
	IgnoreFailure                = 1
	// BeforeExecution means "remove a message before notification"
	BeforeExecution              = 2
)

// Config configures sqsnotify2 service
type Config struct {
	Profile    string
	Region     string
	QueueName  string
	MaxRetries int

	CacheName string

	Workers       int
	RemovePolicy  RemovePolicy
	CmdName       string
	CmdArgs       []string

	Logger *log.Logger
}

// NewConfig creates a new Config object.
func NewConfig() *Config {
	return &Config{
		Region:  "us-east-1",
		Workers: runtime.NumCPU(),
	}
}
