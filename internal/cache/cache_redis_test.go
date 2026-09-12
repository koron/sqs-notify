package cache

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/koron/sqs-notify/v2/internal/stage"
)

func TestRedisCache_DefaultLifetime(t *testing.T) {
	m := miniredis.RunT(t)
	s := fmt.Sprintf("redis://%s/?prefix=default_", m.Addr())
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("failed to parse as URL %q: %v", s, err)
	}

	rc, err := newRedisCache(context.Background(), u)
	if err != nil {
		t.Fatalf("failed to create redisCache: %v", err)
	}
	t.Cleanup(func() { rc.Close() })

	if rc.lifetime != time.Hour {
		t.Errorf("unexpected default lifetime: want=%v got=%v", time.Hour, rc.lifetime)
	}

	err = rc.Insert("msg1", stage.Recv)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	ttl := m.TTL("default_msg1")
	if ttl != time.Hour {
		t.Errorf("unexpected TTL in redis: want=%v got=%v", time.Hour, ttl)
	}
}

func TestRedisCache_CustomLifetime(t *testing.T) {
	m := miniredis.RunT(t)
	s := fmt.Sprintf("redis://%s/?prefix=custom_&lifetime=10s", m.Addr())
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("failed to parse as URL %q: %v", s, err)
	}

	rc, err := newRedisCache(context.Background(), u)
	if err != nil {
		t.Fatalf("failed to create redisCache: %v", err)
	}
	t.Cleanup(func() { rc.Close() })

	if rc.lifetime != 10*time.Second {
		t.Errorf("unexpected lifetime: want=%v got=%v", 10*time.Second, rc.lifetime)
	}

	err = rc.Insert("msg1", stage.Recv)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	ttl := m.TTL("custom_msg1")
	if ttl != 10*time.Second {
		t.Errorf("unexpected TTL in redis: want=%v got=%v", 10*time.Second, ttl)
	}
}

func TestRedisCache_LifecycleAndExpiration(t *testing.T) {
	m := miniredis.RunT(t)
	s := fmt.Sprintf("redis://%s/?prefix=exp_&lifetime=5s", m.Addr())
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("failed to parse as URL %q: %v", s, err)
	}

	rc, err := newRedisCache(context.Background(), u)
	if err != nil {
		t.Fatalf("failed to create redisCache: %v", err)
	}
	t.Cleanup(func() { rc.Close() })

	// Standard lifecycle checks
	testCache(t, rc)

	// Expiration check
	err = rc.Insert("expire_me", stage.Recv)
	if err != nil {
		t.Fatalf("failed to insert key: %v", err)
	}

	if !m.Exists("exp_expire_me") {
		t.Fatalf("expected key to exist in miniredis")
	}

	// Advance time past lifetime
	m.FastForward(6 * time.Second)

	if m.Exists("exp_expire_me") {
		t.Fatalf("expected key to be expired in miniredis")
	}

	// Re-insertion after expiration should succeed
	err = rc.Insert("expire_me", stage.Recv)
	if err != nil {
		t.Fatalf("expected re-insertion to succeed after expiration, got: %v", err)
	}
}

func TestRedisCache_NewCache(t *testing.T) {
	m := miniredis.RunT(t)
	s := fmt.Sprintf("redis://%s/1?prefix=nc_&lifetime=15s", m.Addr())

	c, err := NewCache(context.Background(), s)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}
	defer c.Close()

	rc, ok := c.(*redisCache)
	if !ok {
		t.Fatalf("unexpected cache type: %T", c)
	}
	if rc.lifetime != 15*time.Second {
		t.Errorf("unexpected lifetime: want=%v got=%v", 15*time.Second, rc.lifetime)
	}
	if rc.prefix != "nc_" {
		t.Errorf("unexpected prefix: want=%s got=%s", "nc_", rc.prefix)
	}
}
