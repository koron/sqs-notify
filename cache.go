package main

import (
	"container/list"
	"crypto/md5"
	"fmt"
	"sync"
)

type cache interface {
	AddTry(s string) bool
	Delete(s string)
	Complete(s string)
	IsComplete(s string) bool
}

func newCache(capacity int) cache {
	return &digestCache{
		capacity: capacity,
		table:    make(map[string]*list.Element),
		keys:     list.New(),
	}
}

type digestCache struct {
	capacity int
	lock     sync.Mutex
	table    map[string]*list.Element
	keys     *list.List
}

func digest(s string) string {
	h := md5.New()
	return fmt.Sprintf("%x", h.Sum([]byte(s)))
}

func (c *digestCache) AddTry(s string) bool {
	if c.capacity == 0 {
		return true
	}
	k := digest(s)
	// get lock for table.
	c.lock.Lock()
	defer c.lock.Unlock()
	// search from cached key.
	_, ok := c.table[k]
	if ok {
		return false
	}
	// remove old entries, if over capacity.
	for c.keys.Len() >= c.capacity {
		f := c.keys.Front()
		delete(c.table, f.Value.(string))
		c.keys.Remove(f)
	}
	// add a key.
	e := c.keys.PushBack(k)
	c.table[k] = e
	return true
}

func (c *digestCache) Delete(s string) {
	if c.capacity == 0 {
		return
	}
	k := digest(s)
	// get lock for table.
	c.lock.Lock()
	defer c.lock.Unlock()
	// search cached key.
	e, ok := c.table[k]
	if !ok {
		return
	}
	// remove a key.
	delete(c.table, k)
	c.keys.Remove(e)
}

func (c *digestCache) Complete(s string) {
	if c.capacity == 0 {
		return
	}
	// TODO:
}

func (c *digestCache) IsComplete(s string) bool {
	if c.capacity == 0 {
		return false
	}
	// TODO:
	return false
}
