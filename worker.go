package main

import (
	"os"
	"os/exec"
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
	cmds []*exec.Cmd
}

func newWorkers(num int) *workers {
	jobs := make(chan workerJob, 1)
	w := &workers{
		num:  num,
		jobs: jobs,
		wait: &sync.WaitGroup{},
		cmds: make([]*exec.Cmd, num),
	}

	for i := 0; i < num; i++ {
		go w.startWorker(i, jobs)
	}
	return w
}

func (w *workers) startWorker(num int, jobs chan workerJob) {
	for j := range jobs {
		w.cmds[num] = j.Cmd
		err := j.Cmd.Run()
		res := workerResult{
			Code:         getStatusCode(err),
			Error:        err,
			ProcessState: j.Cmd.ProcessState,
		}
		if j.Finish != nil {
			j.Finish(res)
		}
		w.wait.Done()
		w.cmds[num] = nil
	}
}

func (w *workers) Run(job workerJob) {
	w.wait.Add(1)
	w.jobs <- job
}

func (w *workers) Wait() {
	w.wait.Wait()
}

func (w *workers) Kill() {
	for _, c := range w.cmds {
		if c == nil || c.Process == nil {
			continue
		}
		c.Process.Kill()
	}
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
