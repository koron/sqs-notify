package sqsnotify2

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/go-redis/redis"
	"github.com/koron/sqs-notify/sqsnotify2/stage"
)

type redisCache struct {
	c *redis.Client

	prefix   string
	lifetime time.Duration
}

func newRedisCache(ctx context.Context, u *url.URL) (*redisCache, error) {
	if u.Scheme != "redis" {
		return nil, fmt.Errorf("unexpected scheme: %s", u.Scheme)
	}
	var (
		err error
		opt = &redis.Options{Addr: u.Host}
		rc  = &redisCache{}
	)
	if u.User != nil {
		opt.Password, _ = u.User.Password()
	}
	if u.Path != "" {
		opt.DB, err = strconv.Atoi(u.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to parse path as DB num: %s", err)
		}
	}
	v := u.Query()
	if s := v.Get("lifetime"); s != "" {
		rc.lifetime, err = time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("failed to parse lifetime: %s", err)
		}
	}
	if s := v.Get("prefix"); s != "" {
		rc.prefix = s
	}
	c := redis.NewClient(opt).WithContext(ctx)
	if _, err := c.Ping().Result(); err != nil {
		if err != nil {
			c.Close()
			return nil, fmt.Errorf("failed to ping: %s", err)
		}
	}
	rc.c = c
	return rc, nil
}

func (rc *redisCache) key(id string) string {
	return rc.prefix + id
}

func (rc *redisCache) Insert(id string, stg stage.Stage) error {
	if stg == stage.None {
		return nil
	}
	b, err := rc.c.SetNX(rc.key(id), stg, rc.lifetime).Result()
	if err != nil {
		return err
	}
	if !b {
		return errCacheFound
	}
	return nil
}

func (rc *redisCache) Update(id string, stg stage.Stage) error {
	if stg == stage.None {
		return nil
	}
	b, err := rc.c.SetXX(rc.key(id), stg, rc.lifetime).Result()
	if err != nil {
		return err
	}
	if !b {
		return errCacheNotFound
	}
	return nil
}

func (rc *redisCache) Delete(id string) error {
	_, err := rc.c.Del(rc.key(id)).Result()
	if err != nil {
		return err
	}
	return nil
}

func (rc *redisCache) Close() error {
	if rc.c != nil {
		err := rc.c.Close()
		rc.c = nil
		return err
	}
	return nil
}
