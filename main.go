package main

import (
	"errors"
	"fmt"
	"github.com/koron/sqs-notify/sqsnotify"
	"io"
	"launchpad.net/goamz/aws"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const progname = "sqs-notify"

type app struct {
	auth   aws.Auth
	region aws.Region
	worker int
	nowait bool
	// retryMax is max count of retry.
	retryMax int
	notify   *sqsnotify.SQSNotify
	cmd      string
	args     []string
}

func usage() {
	fmt.Printf(`Usage: %s [OPTIONS] {queue name} {command and args...}

OPTIONS:
  -region {region} :    name of region (default: us-east-1)
  -worker {num} :       num of workers (default: 4)
  -nowait :             didn't wait end of command to delete message
  -retrymax {num} :     num of retry count (default: 4)
  -logfile {path} :     log file path
  -pidfile {path} :     pid file path (available with -logfile)

Environment variables:
  AWS_ACCESS_KEY_ID
  AWS_SECRET_ACCESS_KEY
`, progname)
	os.Exit(1)
}

func retryDuration(c int) time.Duration {
	limit := (1 << uint(c)) - 1
	if limit > 50 {
		limit = 50
	}
	v := rand.Intn(limit)
	return time.Duration(v*200) * time.Millisecond
}

func (a *app) run() (err error) {
	// Open a queue.
	err = a.notify.Open()
	if err != nil {
		return
	}

	// Listen queue.
	c, err := a.notify.Listen()
	if err != nil {
		return
	}

	w := NewWorkers(a.worker)

	// Receive *sqsnotify.SQSMessage via channel.
	retryCount := 0
	for m := range c {
		if m.Error != nil {
			if retryCount >= a.retryMax {
				log.Println("sqs-notify (abort):", m.Error)
				return errors.New("Over retry: " + strconv.Itoa(retryCount))
			} else {
				log.Println("sqs-notify (retry):", m.Error)
				retryCount += 1
				time.Sleep(retryDuration(retryCount))
				// TODO: sleep before retry.
				continue
			}
		} else {
			retryCount = 0
		}

		// Create and setup a exec.Cmd.
		cmd := exec.Command(a.cmd, a.args...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}

		if a.nowait {
			m.Delete() // FIXME: log it when failed to delete.
			w.Run(WorkerJob{cmd, nil})
		} else {
			w.Run(WorkerJob{cmd, func(r WorkerResult) {
				if r.ProcessState != nil && r.ProcessState.Success() {
					m.Delete() // FIXME: log it when failed to delete.
				}
			}})
		}
		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)
		go func() {
			stdin.Write([]byte(*m.Body()))
			stdin.Close()
		}()
	}

	return
}

func main() {
	c, err := getConfig()
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}

	a, err := c.toApp()
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}

	err = a.run()
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}
}
