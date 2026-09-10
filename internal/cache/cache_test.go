package cache

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/koron/sqs-notify/internal/stage"
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
	m := miniredis.RunT(t)
	s := fmt.Sprintf("redis://%s/?prefix=%s&lifetime=10s", m.Addr(), t.Name())
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("failed to parse as URL %q: %v", s, err)
	}

	rc, err := newRedisCache(context.Background(), u)
	if err != nil {
		t.Fatalf("failed to create redisCache: %v", err)
	}
	t.Cleanup(func() { rc.Close() })

	testCache(t, rc)
}

func TestMemoryCache(t *testing.T) {
	mc := NewMemoryCache(minCapacity)
	testCache(t, mc)
}

func TestNewCacheMemory(t *testing.T) {
	c, err := NewCache(context.Background(), "memory://?capacity=1234")
	if err != nil {
		t.Fatal(err)
	}

	mc, ok := c.(*memoryCache)
	if !ok {
		t.Fatalf("unexpected cache type: %T", c)
	}
	if mc.c != 1234 {
		t.Errorf("unexpected capacity: want=%d got=%d", 1234, mc.c)
	}

	testCache(t, mc)
}
