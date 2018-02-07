package sqsnotify2

import (
	"container/list"
	"context"
	"fmt"
	"net/url"
	"strconv"
	"sync"

	"github.com/koron/sqs-notify/sqsnotify2/stage"
)

type cache interface {
	Put(ctx *context.Context, id string, stg stage.Stage) error
	Get(ctx *context.Context, id string) (stage.Stage, error)
	Delete(ctx *context.Context, id string) error
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

func (mc *memoryCache) Put(_ *context.Context, id string, stg stage.Stage) error {
	mc.l.Lock()
	defer mc.l.Unlock()

	if mc.c <= 0 {
		return nil
	}
	if stg == stage.None {
		return nil
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

func (mc *memoryCache) Get(_ *context.Context, id string) (stage.Stage, error) {
	mc.l.Lock()
	defer mc.l.Unlock()

	if mc.c <= 0 {
		return stage.None, nil
	}
	v, ok := mc.m[id]
	if !ok {
		return stage.None, nil
	}
	return v.stg, nil
}

func (mc *memoryCache) Delete(_ *context.Context, id string) error {
	mc.l.Lock()
	defer mc.l.Unlock()

	if mc.c <= 0 {
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

func newCache(name string) (cache, error) {
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
		// TODO: implement me.
	}
	return nil, fmt.Errorf("not supported cache: %s", name)
}
