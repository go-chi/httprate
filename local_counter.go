package httprate

import (
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

var _ LimitCounter = &localCounter{}

type localCounter struct {
	latestWindow     time.Time
	previousCounters map[uint64]int
	latestCounters   map[uint64]int
	windowLength     time.Duration
	mu               sync.RWMutex
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

	c.evict(currentWindow)

	hkey := limitCounterKey(key, currentWindow)

	count, _ := c.latestCounters[hkey]
	c.latestCounters[hkey] = count + amount

	return nil
}

func (c *localCounter) Get(key string, currentWindow, previousWindow time.Time) (int, int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.latestWindow == currentWindow {
		curr, _ := c.latestCounters[limitCounterKey(key, currentWindow)]
		prev, _ := c.previousCounters[limitCounterKey(key, previousWindow)]
		return curr, prev, nil
	}

	if c.latestWindow == previousWindow {
		prev, _ := c.latestCounters[limitCounterKey(key, previousWindow)]
		return 0, prev, nil
	}

	return 0, 0, nil
}

func (c *localCounter) evict(currentWindow time.Time) {
	if c.latestWindow == currentWindow {
		return
	}

	previousWindow := currentWindow.Add(-c.windowLength)
	if c.latestWindow == previousWindow {
		c.latestWindow = currentWindow
		c.latestCounters, c.previousCounters = make(map[uint64]int), c.latestCounters
		return
	}

	c.latestWindow = currentWindow
	// NOTE: Don't use clear() to keep backward-compatibility.
	c.previousCounters, c.latestCounters = make(map[uint64]int), make(map[uint64]int)
}

func limitCounterKey(key string, window time.Time) uint64 {
	h := xxhash.New()
	h.WriteString(key)
	return h.Sum64()
}
