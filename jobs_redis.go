package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"gopkg.in/redis.v3"
)

type redisJobsOptions struct {
	redis.Options
	KeyPrefix  string
	Expiration string
}

type redisJobsManager struct {
	logger     *log.Logger
	lock       sync.Mutex
	client     *redis.Client
	keyPrefix  string
	expiration time.Duration
}

func newRedisJobs(opt redisJobsOptions) (jobs, error) {
	expiration, err := time.ParseDuration(opt.Expiration)
	if err != nil {
		return nil, err
	}
	c := redis.NewClient(&opt.Options)
	if c == nil {
		return nil, fmt.Errorf("redis.NewClient failed: %#v", opt.Options)
	}
	// check connection.
	if err := c.Ping().Err(); err != nil {
		c.Close()
		return nil, err
	}
	m := &redisJobsManager{
		client:     c,
		keyPrefix:  opt.KeyPrefix,
		expiration: expiration,
	}
	return m, nil
}

func (m *redisJobsManager) StartTry(id string) jobState {
	m.lock.Lock()
	defer m.lock.Unlock()
	s, ok := m.get(id)
	if ok {
		return s
	}
	m.insert(id, jobRunning)
	return jobStarted
}

func (m *redisJobsManager) Fail(id string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.remove(id)
}

func (m *redisJobsManager) Complete(id string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.update(id, jobCompleted)
}

func (m *redisJobsManager) Close() {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}
}

func (m *redisJobsManager) logNilCmd(cmd string) {
	m.logErr(fmt.Errorf("command %s returns nil", cmd))
}

func (m *redisJobsManager) logCmdErr(cmd string, err error) {
	m.logErr(fmt.Errorf("command %s returns error: %v", cmd, err))
}

func (m *redisJobsManager) logErr(err error) {
	if m.logger == nil {
		return
	}
	m.logger.Printf("REDIS: %s", err)
}

func (m *redisJobsManager) key(id string) string {
	return m.keyPrefix + id
}

func (m *redisJobsManager) insert(id string, s jobState) bool {
	c := m.client.SetNX(m.key(id), s.String(), m.expiration)
	if c == nil {
		m.logNilCmd("SetNX")
		return false
	}
	b, err := c.Result()
	if err != nil {
		m.logCmdErr("SetNX", err)
		return false
	}
	return b
}

func (m *redisJobsManager) update(id string, s jobState) bool {
	c := m.client.SetXX(m.key(id), s.String(), m.expiration)
	if c == nil {
		m.logNilCmd("SetXX")
		return false
	}
	b, err := c.Result()
	if err != nil {
		m.logCmdErr("SetXX", err)
		return false
	}
	return b
}

func (m *redisJobsManager) remove(id string) bool {
	c := m.client.Del(m.key(id))
	if c == nil {
		m.logNilCmd("Del")
		return false
	}
	n, err := c.Result()
	if err != nil {
		m.logCmdErr("Del", err)
	}
	if n != 1 {
		return false
	}
	return true
}

func (m *redisJobsManager) get(id string) (jobState, bool) {
	c := m.client.Get(m.key(id))
	if c == nil {
		m.logNilCmd("Get")
		return 0, false
	}
	v, err := c.Result()
	if err != nil {
		m.logCmdErr("Get", err)
		return 0, false
	}
	s, ok := parseJobState(v)
	if !ok {
		m.logErr(fmt.Errorf("uknown state: %#v", v))
		return 0, false
	}
	return s, true
}
