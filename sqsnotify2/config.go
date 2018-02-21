package sqsnotify2

import (
	"log"
	"runtime"
)

type RemovePolicy int

const (
	Succeed         RemovePolicy = 0
	IgnoreFailure                = 1
	BeforeExecution              = 2
)

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
