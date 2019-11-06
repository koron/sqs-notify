package sqsnotify2

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/koron/sqs-notify/sqsnotify2/stage"
)

func testCache(t *testing.T, c Cache) {
	id1 := "1234"
	id2 := "abcd"

	err := c.Insert(id1, stage.Recv)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	err = c.Insert(id1, stage.Recv)
	if err != errCacheFound {
		t.Fatalf("unexpected insertion: %v", err)
	}

	err = c.Update(id1, stage.Exec)
	if err != nil {
		t.Fatalf("failed to update: %v", err)
	}
	err = c.Update(id2, stage.Exec)
	if err != errCacheNotFound {
		t.Fatalf("unexpected update: %v", err)
	}

	err = c.Delete(id1)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}
	err = c.Delete(id1)
	if err != nil {
		t.Fatalf("failed to delete none: %v", err)
	}
}

func TestRedisCache(t *testing.T) {
	s := os.Getenv("REDIS_URL")
	if s == "" {
		t.Skip("skipping test because REDIS_URL isn't given")
		return
	}
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("failed to parse REDIS_URL: %v", err)
	}
	rc, err := newRedisCache(context.Background(), u)
	if err != nil {
		t.Fatalf("failed to create redisCache: %v", err)
	}
	defer rc.Close()
	rc.prefix = t.Name()
	rc.lifetime = 10 * time.Second

	testCache(t, rc)
}

func TestMemoryCache(t *testing.T) {
	mc := newMemoryCache(minCapacity)
	testCache(t, mc)
}
