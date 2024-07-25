//go:build go1.21

package httprate

import "time"

func (c *localCounter) evict(currentWindow time.Time) {
	if c.latestWindow == currentWindow {
		return
	}

	previousWindow := currentWindow.Add(-c.windowLength)
	if c.latestWindow == previousWindow {
		c.latestWindow = currentWindow
		// Shift the windows without map re-allocation.
		clear(c.previousCounters)
		c.latestCounters, c.previousCounters = c.previousCounters, c.latestCounters
		return
	}

	c.latestWindow = currentWindow

	clear(c.previousCounters)
	clear(c.latestCounters)
}
