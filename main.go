package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/koron/sqs-notify/sqsnotify"
	"launchpad.net/goamz/aws"
)

const progname = "sqs-notify"

type app struct {
	logger   *log.Logger
	auth     aws.Auth
	region   aws.Region
	worker   int
	nowait   bool
	retryMax int
	msgCache cache
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
  -msgcache {num} :     num of last received message in cache (default: 0)
  -logfile {path} :     log file path ("-" for stdout)
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

func (a *app) log(v ...interface{}) {
	if a.logger == nil {
		return
	}
	a.logger.Print(v)
}

func (a *app) logOk(m *sqsnotify.SQSMessage, r workerResult) {
	if a.logger == nil {
		return
	}
	// Log as OK.
	a.logger.Printf("\tEXECUTED\tqueue:%s\tbody:%#v\tcmd:%s\tstatus:%d",
		a.notify.Name(), *m.Body(), a.cmd, r.Code)
}

func (a *app) logNg(m *sqsnotify.SQSMessage, err error) {
	if a.logger == nil {
		return
	}
	a.logger.Printf("\tNOT_EXECUTED\tqueue:%s\tbody:%#v\terror:%s",
		a.notify.Name(), *m.Body(), err)
}

func (a *app) deleteSQSMessage(m *sqsnotify.SQSMessage) {
	// FIXME: log it when failed to delete.
	m.Delete()
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

	w := newWorkers(a.worker)

	// Receive *sqsnotify.SQSMessage via channel.
	retryCount := 0
	for m := range c {
		if m.Error != nil {
			if retryCount >= a.retryMax {
				a.log("abort:", m.Error)
				log.Println("sqs-notify (abort):", m.Error)
				return errors.New("Over retry: " + strconv.Itoa(retryCount))
			}
			a.log("retry:", m.Error)
			log.Println("sqs-notify (retry):", m.Error)
			retryCount++
			// sleep before retry.
			time.Sleep(retryDuration(retryCount))
			continue
		} else {
			retryCount = 0
		}

		if !a.msgCache.AddTry(*m.Body()) {
			continue
		}

		// Create and setup a exec.Cmd.
		cmd := exec.Command(a.cmd, a.args...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			a.logNg(m, err)
			a.msgCache.Delete(*m.Body())
			return err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			a.logNg(m, err)
			a.msgCache.Delete(*m.Body())
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			a.logNg(m, err)
			a.msgCache.Delete(*m.Body())
			return err
		}

		if a.nowait {
			a.deleteSQSMessage(m)
			w.Run(workerJob{cmd, func(r workerResult) {
				a.logOk(m, r)
				if !r.Success() {
					a.msgCache.Delete(*m.Body())
				}
			}})
		} else {
			w.Run(workerJob{cmd, func(r workerResult) {
				a.logOk(m, r)
				if r.Success() {
					a.deleteSQSMessage(m)
				} else {
					a.msgCache.Delete(*m.Body())
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
