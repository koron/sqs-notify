package main

import (
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
)

type WorkerResult struct {
	Error        error
	Code         int
	ProcessState *os.ProcessState
}

func (r *WorkerResult) Success() bool {
	return r.ProcessState != nil && r.ProcessState.Success()
}

type WorkerJob struct {
	Cmd    *exec.Cmd
	Finish func(WorkerResult)
}

type Workers struct {
	num  int
	jobs chan WorkerJob
	wait *sync.WaitGroup
}

func NewWorkers(num int) *Workers {
	jobs := make(chan WorkerJob, 100)
	w := &Workers{num, jobs, &sync.WaitGroup{}}

	for i := 0; i < num; i++ {
		go w.startWorker(i, jobs)
	}
	return w
}

func (w *Workers) startWorker(num int, jobs chan WorkerJob) {
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

		res := WorkerResult{
			Code: getStatusCode(err),
			Error: err,
			ProcessState: j.Cmd.ProcessState,
		}
		if j.Finish != nil {
			j.Finish(res)
		}
		w.wait.Done()
	}
}

func (w *Workers) Run(job WorkerJob) {
	w.wait.Add(1)
	w.jobs <- job
}

func (w *Workers) Wait() {
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
