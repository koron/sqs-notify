package main

type cache interface {
	AddTry(s string) bool
	Delete(s string)
}

func newCache(size int) cache {
	return &digestCache{
		size: size,
	}
}

type digestCache struct {
	size int
}

func (c *digestCache) AddTry(s string) bool {
	if c.size == 0 {
		return true
	}
	// TODO:
	return true
}

func (c *digestCache) Delete(s string) {
	// TODO:
}
