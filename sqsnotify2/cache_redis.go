package sqsnotify2

import (
	"context"
	"net/url"

	"github.com/go-redis/redis"
	"github.com/koron/sqs-notify/sqsnotify2/stage"
)

type redisCache struct {
	c *redis.Client
}

func newRedisCache(u *url.URL, ctx context.Context) (*redisCache, error) {
	var c *redis.Client
	// TODO:
	return &redisCache{
		c: c.WithContext(ctx),
	}, nil
}

func (rc *redisCache) Insert(id string, stg stage.Stage) error {
	// TODO:
	return nil
}

func (rc *redisCache) Update(id string, stg stage.Stage) error {
	// TODO:
	return nil
}

func (rc *redisCache) Delete(id string) error {
	// TODO:
	return nil
}
