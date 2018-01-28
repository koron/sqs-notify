package main

import (
	"errors"
	"flag"
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
	"github.com/goamz/goamz/sqs"
	"github.com/koron/sqs-notify/sqsnotify"
)

const progname = "sqs-notify"

var (
	version  = "1.5.6"
	revision = ""
)

type app struct {
	logger        *log.Logger
	auth          aws.Auth
	region        aws.Region
	worker        int
	nowait        bool
	ignoreFailure bool
	messageCount  int
	digestID      bool
	retryMax      int
	jobs          jobs
	notify        *sqsnotify.SQSNotify
	cmd           string
	args          []string

	w *workers
}

func showVersion() {
	v := version
	if revision != "" {
		v = fmt.Sprintf("%s (rev:%s)", version, revision)
	}
	fmt.Printf("%s version %s\n", progname, v)
	os.Exit(1)
}

func usage() {
	fmt.Printf(`Usage: %s [OPTIONS] {queue name} {command and args...}

OPTIONS:
`, progname)
	flag.PrintDefaults()
	fmt.Println("\nSource: https://github.com/koron/sqs-notify")
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
	a.logger.Print(v...)
}

func (a *app) logf(s string, args ...interface{}) {
	if a.logger == nil {
		return
	}
	a.logger.Printf(s, args...)
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

func (a *app) logAbort(err error) {
	s := a.errorSQS(err)
	a.log("abort:", s)
	log.Println("sqs-notify (abort):", s)
}

func (a *app) logRetry(err error) {
	s := a.errorSQS(err)
	a.log("retry:", s)
	log.Println("sqs-notify (retry):", s)
}

func (a *app) errorSQS(err error) string {
	switch err := err.(type) {
	case *sqs.Error:
		return fmt.Sprintf("%s (Code:%s, RequestId:%s)",
			err.Message, err.Code, err.RequestId)
	default:
		return err.Error()
	}
}

func (a *app) deleteSQSMessage(m *sqsnotify.SQSMessage) {
	a.notify.ReserveDelete(m)
}

func (a *app) messageID(m *sqsnotify.SQSMessage) string {
	if a.digestID {
		return m.Message.MD5OfBody
	}
	return m.ID()
}

func (a *app) run() (err error) {
	// Open a queue.
	sqsnotify.MessageCount = a.messageCount
	err = a.notify.Open()
	if err != nil {
		return
	}

	// Listen queue.
	c, err := a.notify.Listen()
	if err != nil {
		return
	}
	defer a.notify.Stop()

	// accept CTRL+C to terminate.
	sig := make(chan os.Signal, 1)
	go func() {
		for {
			s := <-sig
			if s == os.Interrupt {
				break
			}
		}
		signal.Stop(sig)
		close(sig)
		close(c)
	}()
	signal.Notify(sig, os.Interrupt)

	a.w = newWorkers(a.worker)
	defer a.waitWorkers()

	// Receive *sqsnotify.SQSMessage via channel.
	retryCount := 0
	for m := range c {
		if m.Error != nil {
			if retryCount >= a.retryMax {
				a.logAbort(m.Error)
				return errors.New("over retry: " + strconv.Itoa(retryCount))
			}
			a.logRetry(m.Error)
			retryCount++
			// sleep before retry.
			time.Sleep(retryDuration(retryCount))
			continue
		} else {
			retryCount = 0
		}

		body := *m.Body()
		jid := a.messageID(m)
		st, err := a.jobs.StartTry(jid)
		if err != nil {
			return fmt.Errorf("failed to register/start a job: %s", err)
		}
		switch st {
		case jobRunning:
			a.logSkip(body)
			if a.nowait {
				a.deleteSQSMessage(m)
			}
			continue
		case jobCompleted:
			a.logSkip(body)
			a.deleteSQSMessage(m)
			continue
		}

		// Create and setup a exec.Cmd.
		if err := a.execCmd(m, jid, body); err != nil {
			a.logNg(body, err)
			a.jobs.Fail(jid)
		}
	}

	return
}

func (a *app) waitWorkers() {
	a.w.Wait()
}

func (a *app) execCmd(m *sqsnotify.SQSMessage, jid, body string) error {
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
		a.deleteSQSMessage(m)
		a.w.Run(workerJob{cmd, func(r workerResult) {
			a.logOk(body, r)
			if r.Success() || a.ignoreFailure {
				a.jobs.Complete(jid)
			} else {
				a.jobs.Fail(jid)
			}
		}})
	} else {
		a.w.Run(workerJob{cmd, func(r workerResult) {
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
		_, err := stdin.Write([]byte(body))
		if err != nil {
			a.logf("\tWARN: failed to write body\tID:%s\tBODY:%s",
				m.Message.MessageId, body)
		}
		_ = stdin.Close()
	}()

	return nil
}

func main() {
	c, err := getConfig()
	if err != nil {
		log.Fatalln("sqs-notify:", err)
	}

	if c.daemon {
		makeDaemon()
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
