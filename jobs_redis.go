package main

import (
	"gopkg.in/redis.v3"
)

type redisJobsManager struct {
	client *redis.Client
}

func newRedisJobs(opt *redis.Options) (jobs, error) {
	c := redis.NewClient(opt)
	// TODO: check connection.
	return &redisJobsManager{
		client: c,
	}, nil
}

func (m *redisJobsManager) StartTry(id string) jobState {
	// TODO:
	return jobStarted
}

func (m *redisJobsManager) Fail(id string) {
	// TODO:
}

func (m *redisJobsManager) Complete(id string) {
	// TODO:
}

func (m *redisJobsManager) Close() {
	// FIXME: use critical section
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}
}
