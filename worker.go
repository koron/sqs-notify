package main

import (
	"os"
	"os/exec"
	"os/signal"
	"sync"
)

type WorkerResult struct {
	Error error
	State *os.ProcessState
}

type WorkerJob struct {
	cmd *exec.Cmd
	finish func(WorkerResult)
}

type Workers struct {
	num int
	jobs chan WorkerJob
	wait *sync.WaitGroup
}

func NewWorkers(num int) (*Workers) {
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
		signal.Notify(sig, os.Interrupt)
		go func() {
			switch <-sig {
			case os.Interrupt:
				j.cmd.Process.Kill()
			}
		}()
		err := j.cmd.Run()
		close(sig)
		res := WorkerResult{err, j.cmd.ProcessState}
		if j.finish != nil {
			j.finish(res)
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
