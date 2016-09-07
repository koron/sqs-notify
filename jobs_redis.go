package main

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"gopkg.in/redis.v3"
)

var (
	errNilCmd = errors.New("redis client returned nil response")
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

func newRedisJobs(opt redisJobsOptions) (*redisJobsManager, error) {
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
		_ = c.Close()
		return nil, err
	}
	return &redisJobsManager{
		client:     c,
		keyPrefix:  opt.KeyPrefix,
		expiration: expiration,
	}, nil
}

func (m *redisJobsManager) StartTry(id string) (jobState, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	s, ok, err := m.get(id)
	if err != nil {
		return 0, err
	}
	if ok {
		return s, nil
	}
	if _, err := m.insert(id, jobRunning); err != nil {
		return 0, err
	}
	return jobStarted, nil
}

func (m *redisJobsManager) Fail(id string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	// all errors are logged in m.remove().
	_, _ = m.remove(id)
}

func (m *redisJobsManager) Complete(id string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	// all errors are logged in m.update().
	_, _ = m.update(id, jobCompleted)
}

func (m *redisJobsManager) Close() {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.client != nil {
		_ = m.client.Close()
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

func (m *redisJobsManager) insert(id string, s jobState) (bool, error) {
	c := m.client.SetNX(m.key(id), s.String(), m.expiration)
	if c == nil {
		m.logNilCmd("SetNX")
		return false, errNilCmd
	}
	b, err := c.Result()
	if err != nil {
		if err != redis.Nil {
			m.logCmdErr("SetNX", err)
		} else {
			err = nil
		}
		return false, err
	}
	return b, nil
}

func (m *redisJobsManager) update(id string, s jobState) (bool, error) {
	c := m.client.SetXX(m.key(id), s.String(), m.expiration)
	if c == nil {
		m.logNilCmd("SetXX")
		return false, errNilCmd
	}
	b, err := c.Result()
	if err != nil {
		if err != redis.Nil {
			m.logCmdErr("SetXX", err)
		} else {
			err = nil
		}
		return false, err
	}
	return b, nil
}

func (m *redisJobsManager) remove(id string) (bool, error) {
	c := m.client.Del(m.key(id))
	if c == nil {
		m.logNilCmd("Del")
		return false, errNilCmd
	}
	n, err := c.Result()
	if err != nil {
		if err != redis.Nil {
			m.logCmdErr("Del", err)
		} else {
			err = nil
		}
		return false, err
	}
	if n != 1 {
		return false, nil
	}
	return true, nil
}

func (m *redisJobsManager) get(id string) (jobState, bool, error) {
	c := m.client.Get(m.key(id))
	if c == nil {
		m.logNilCmd("Get")
		return 0, false, errNilCmd
	}
	v, err := c.Result()
	if err != nil {
		if err != redis.Nil {
			m.logCmdErr("Get", err)
		} else {
			err = nil
		}
		return 0, false, err
	}
	s, ok := parseJobState(v)
	if !ok {
		err := fmt.Errorf("uknown state: %#v", v)
		m.logErr(err)
		return 0, false, err
	}
	return s, true, nil
}
