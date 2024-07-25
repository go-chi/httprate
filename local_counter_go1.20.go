//go:build !go1.21

package httprate

import "time"

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
	c.previousCounters, c.latestCounters = make(map[uint64]int), make(map[uint64]int)
}
