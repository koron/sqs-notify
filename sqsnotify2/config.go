package sqsnotify2

import (
	"log"
	"runtime"
)

type Config struct {
	Profile    string
	Region     string
	QueueName  string
	MaxRetries int

	Workers       int
	IgnoreFailure bool
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
