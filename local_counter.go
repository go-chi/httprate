package httprate

import (
	"fmt"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

var _ LimitCounter = &localCounter{}

type localCounter struct {
	counters     map[uint64]*count
	windowLength time.Duration
	lastEvict    time.Time
	mu           sync.Mutex
}

type count struct {
	value     int
	updatedAt time.Time
}

func (c *localCounter) Config(requestLimit int, windowLength time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.windowLength = windowLength
}

func (c *localCounter) Increment(key string, currentWindow time.Time) error {
	return c.IncrementBy(key, currentWindow, 1)
}

func (c *localCounter) IncrementBy(key string, currentWindow time.Time, amount int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.evict()

	hkey := LimitCounterKey(key, currentWindow)

	v, ok := c.counters[hkey]
	if !ok {
		v = &count{}
		c.counters[hkey] = v
	}
	v.value += amount
	v.updatedAt = time.Now()

	return nil
}

func (c *localCounter) Get(key string, currentWindow, previousWindow time.Time) (int, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	curr, ok := c.counters[LimitCounterKey(key, currentWindow)]
	if !ok {
		curr = &count{value: 0, updatedAt: time.Now()}
	}
	prev, ok := c.counters[LimitCounterKey(key, previousWindow)]
	if !ok {
		prev = &count{value: 0, updatedAt: time.Now()}
	}

	return curr.value, prev.value, nil
}

func (c *localCounter) evict() {
	d := c.windowLength * 3

	if time.Since(c.lastEvict) < d {
		return
	}
	c.lastEvict = time.Now()

	for k, v := range c.counters {
		if time.Since(v.updatedAt) >= d {
			delete(c.counters, k)
		}
	}
}

func LimitCounterKey(key string, window time.Time) uint64 {
	h := xxhash.New()
	h.WriteString(key)
	h.WriteString(fmt.Sprintf("%d", window.Unix()))
	return h.Sum64()
}
