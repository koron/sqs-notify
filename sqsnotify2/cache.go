package sqsnotify2

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"

	"github.com/koron/sqs-notify/sqsnotify2/stage"
)

const minCapacity = maxMsg

var (
	errCacheFound    = errors.New("cache found")
	errCacheNotFound = errors.New("cache not found")
)

type cache interface {
	Insert(id string, stg stage.Stage) error
	Update(id string, stg stage.Stage) error
	Delete(id string) error
}

type memoryCache struct {
	c int
	l sync.Mutex
	m map[string]mcEntry
	k *list.List
}

type mcEntry struct {
	el  *list.Element
	stg stage.Stage
}

func newMemoryCache(capacity int) *memoryCache {
	return &memoryCache{
		c: capacity,
		m: make(map[string]mcEntry),
		k: list.New(),
	}
}

func (mc *memoryCache) Insert(id string, stg stage.Stage) error {
	mc.l.Lock()
	defer mc.l.Unlock()

	if stg == stage.None {
		return nil
	}
	if mc.c < minCapacity {
		return nil
	}
	_, ok := mc.m[id]
	if ok {
		return errCacheFound
	}
	// remove old entries.
	for mc.k.Len() >= mc.c {
		f := mc.k.Front()
		delete(mc.m, f.Value.(string))
		mc.k.Remove(f)
	}
	// add an entry.
	el := mc.k.PushBack(id)
	mc.m[id] = mcEntry{el: el, stg: stg}
	return nil
}

func (mc *memoryCache) Update(id string, stg stage.Stage) error {
	mc.l.Lock()
	defer mc.l.Unlock()

	if stg == stage.None {
		return nil
	}
	if mc.c < minCapacity {
		return nil
	}
	v, ok := mc.m[id]
	if !ok {
		return errCacheNotFound
	}
	v.stg = stg
	return nil
}

func (mc *memoryCache) Delete(id string) error {
	mc.l.Lock()
	defer mc.l.Unlock()

	if mc.c < minCapacity {
		return nil
	}
	v, ok := mc.m[id]
	if !ok {
		return nil
	}
	delete(mc.m, id)
	mc.k.Remove(v.el)
	return nil
}

func newCache(ctx context.Context, name string) (cache, error) {
	u, err := url.Parse(name)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "", "memory":
		q := u.Query()
		var capacity int
		s := q.Get("capacity")
		if s != "" {
			capacity, err = strconv.Atoi(s)
			if err != nil {
				return nil, err
			}
		}
		return newMemoryCache(capacity), nil

	case "redis":
		// TODO: close redis cache correctly.
		return newRedisCache(u, ctx)
	}
	return nil, fmt.Errorf("not supported cache: %s", name)
}
