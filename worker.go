package main

import (
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
)

type workerResult struct {
	Error        error
	Code         int
	ProcessState *os.ProcessState
}

func (r *workerResult) Success() bool {
	return r.ProcessState != nil && r.ProcessState.Success()
}

type workerJob struct {
	Cmd    *exec.Cmd
	Finish func(workerResult)
}

type workers struct {
	num  int
	jobs chan workerJob
	wait *sync.WaitGroup
}

func newWorkers(num int) *workers {
	jobs := make(chan workerJob, 100)
	w := &workers{num, jobs, &sync.WaitGroup{}}

	for i := 0; i < num; i++ {
		go w.startWorker(i, jobs)
	}
	return w
}

func (w *workers) startWorker(num int, jobs chan workerJob) {
	for j := range jobs {
		sig := make(chan os.Signal, 1)
		go func() {
			switch <-sig {
			case os.Interrupt:
				j.Cmd.Process.Kill()
			}
		}()

		signal.Notify(sig, os.Interrupt)
		err := j.Cmd.Run()
		signal.Stop(sig)
		close(sig)

		res := workerResult{
			Code:         getStatusCode(err),
			Error:        err,
			ProcessState: j.Cmd.ProcessState,
		}
		if j.Finish != nil {
			j.Finish(res)
		}
		w.wait.Done()
	}
}

func (w *workers) Run(job workerJob) {
	w.wait.Add(1)
	w.jobs <- job
}

func (w *workers) Wait() {
	w.wait.Wait()
}

// Get status code.  It works for Windows and UNIX.
func getStatusCode(err error) int {
	if err != nil {
		if errexit, ok := err.(*exec.ExitError); ok {
			if status, ok := errexit.Sys().(syscall.WaitStatus); ok {
				return status.ExitStatus()
			}
		}
	}
	return 0
}
