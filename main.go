package main

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"time"

	"github.com/goamz/goamz/aws"
	"github.com/koron/sqs-notify/sqsnotify"
)

const progname = "sqs-notify"

type app struct {
	logger        *log.Logger
	auth          aws.Auth
	region        aws.Region
	worker        int
	nowait        bool
	ignoreFailure bool
	retryMax      int
	jobs          jobs
	notify        *sqsnotify.SQSNotify
	cmd           string
	args          []string
}

func usage() {
	fmt.Printf(`Usage: %s [OPTIONS] {queue name} {command and args...}

OPTIONS:
  -region {region} :    name of region (default: us-east-1)
  -worker {num} :       num of workers (default: 4)
  -nowait :             don't wait end of command to delete message
  -ignorefailure:       don't care command results, treat it as success always
  -retrymax {num} :     num of retry count (default: 4)
  -msgcache {num} :     num of last received message in cache (default: 0)
  -redis {path} :       use redis as message cache (default: disabled)
  -logfile {path} :     log file path ("-" for stdout)
  -pidfile {path} :     pid file path (available with -logfile)

  -mode {mode} :        pre-defined set of options for specific usecases

Source: https://github.com/koron/sqs-notify
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

func (a *app) logOk(m string, r workerResult) {
	if a.logger == nil {
		return
	}
	// Log as OK.
	a.logger.Printf("\tEXECUTED\tqueue:%s\tbody:%#v\tcmd:%s\tstatus:%d",
		a.notify.Name(), m, a.cmd, r.Code)
}

func (a *app) logSkip(m string) {
	if a.logger == nil {
		return
	}
	// Log as SKIP.
	a.logger.Printf("\tSKIPPED\tqueue:%s\tbody:%#v\t", a.notify.Name(), m)
}

func (a *app) logNg(m string, err error) {
	if a.logger == nil {
		return
	}
	a.logger.Printf("\tNOT_EXECUTED\tqueue:%s\tbody:%#v\terror:%s",
		a.notify.Name(), m, err)
}

func (a *app) deleteSQSMessage(m *sqsnotify.SQSMessage) {
	// FIXME: log it when failed to delete.
	m.Delete()
}

func (a *app) digest(s string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(s)))
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

	// accept CTRL+C to terminate.
	sig := make(chan os.Signal, 1)
	go func() {
		for {
			switch <-sig {
			case os.Interrupt:
				a.notify.Stop()
				break
			}
		}
		signal.Stop(sig)
		close(sig)
	}()
	signal.Notify(sig, os.Interrupt)

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

		body := *m.Body()
		jid := a.digest(body)
		switch a.jobs.StartTry(jid) {
		case jobRunning:
			a.logSkip(body)
			continue
		case jobCompleted:
			a.logSkip(body)
			a.deleteSQSMessage(m)
			continue
		}

		// Create and setup a exec.Cmd.
		cmd := exec.Command(a.cmd, a.args...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			a.logNg(body, err)
			a.jobs.Fail(jid)
			return err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			a.logNg(body, err)
			a.jobs.Fail(jid)
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			a.logNg(body, err)
			a.jobs.Fail(jid)
			return err
		}

		if a.nowait {
			a.deleteSQSMessage(m)
			w.Run(workerJob{cmd, func(r workerResult) {
				a.logOk(body, r)
				if r.Success() || a.ignoreFailure {
					a.jobs.Complete(jid)
				} else {
					a.jobs.Fail(jid)
				}
			}})
		} else {
			w.Run(workerJob{cmd, func(r workerResult) {
				a.logOk(body, r)
				if r.Success() || a.ignoreFailure {
					a.jobs.Complete(jid)
					a.deleteSQSMessage(m)
				} else {
					a.jobs.Fail(jid)
				}
			}})
		}

		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)
		go func() {
			stdin.Write([]byte(body))
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
	if a.jobs != nil {
		defer a.jobs.Close()
	}

	err = a.run()
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}
}
