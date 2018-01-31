package sqsnotify2

import (
	"runtime"
)

type Config struct {
	Profile    string
	Region     string
	QueueName  string
	MaxRetries int
	Workers    int
	CmdName    string
	CmdArgs    []string
}

// NewConfig creates a new Config object.
func NewConfig() *Config {
	return &Config{
		Region:  "us-east-1",
		Workers: runtime.NumCPU(),
	}
}
