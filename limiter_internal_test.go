package httprate

import (
	"testing"
	"time"
)

// TestCurrentWindowOffset verifies that rate-limit windows are aligned to the
// limiter's sub-window offset rather than to the wall clock, so that resets are
// spread out instead of all snapping to the same instant (e.g. the exact second).
func TestCurrentWindowOffset(t *testing.T) {
	const windowLength = time.Second

	for _, offset := range []time.Duration{
		0,
		347 * time.Millisecond,
		windowLength - time.Nanosecond,
	} {
		l := &RateLimiter{windowLength: windowLength, windowOffset: offset}

		// A time sitting exactly on an offset boundary maps to itself.
		base := time.Unix(1000, 0).UTC().Add(offset)
		if got := l.currentWindow(base); !got.Equal(base) {
			t.Fatalf("offset=%v: currentWindow(boundary) = %v, want %v", offset, got, base)
		}

		// Windows are windowLength apart and aligned to the offset: every window
		// start minus the offset lands exactly on a wall-clock multiple.
		for _, d := range []time.Duration{0, time.Nanosecond, 250 * time.Millisecond, windowLength - time.Nanosecond} {
			now := base.Add(d)
			w := l.currentWindow(now)

			// Result is in (now-windowLength, now].
			if w.After(now) || !w.After(now.Add(-windowLength)) {
				t.Errorf("offset=%v d=%v: currentWindow(%v) = %v out of (now-window, now]", offset, d, now, w)
			}
			// Aligned to the offset, not the wall clock.
			if rem := w.Add(-offset).UnixNano() % int64(windowLength); rem != 0 {
				t.Errorf("offset=%v d=%v: window %v not aligned to offset (rem=%v)", offset, d, w, rem)
			}
			// Same offset-window for any moment inside it.
			if !w.Equal(base) {
				t.Errorf("offset=%v d=%v: currentWindow(%v) = %v, want %v", offset, d, now, w, base)
			}
		}

		// The next window is exactly windowLength later.
		next := l.currentWindow(base.Add(windowLength))
		if want := base.Add(windowLength); !next.Equal(want) {
			t.Errorf("offset=%v: next window = %v, want %v", offset, next, want)
		}
	}
}

// TestLocalCounterWindowOffset verifies the default in-process backend makes the
// limiter derive a sub-window offset, while a custom counter keeps wall-clock-
// aligned windows.
func TestLocalCounterWindowOffset(t *testing.T) {
	const windowLength = time.Second

	// Default limiter uses the local counter and derives a sub-window offset.
	rl := NewRateLimiter(10, windowLength)
	if off := rl.windowOffset; off < 0 || off >= windowLength {
		t.Errorf("limiter windowOffset = %v, want [0, %v)", off, windowLength)
	}

	// A custom counter keeps wall-clock alignment (offset stays zero).
	rl2 := NewRateLimiter(10, windowLength, WithLimitCounter(noOffsetCounter{}))
	if rl2.windowOffset != 0 {
		t.Errorf("limiter windowOffset with custom counter = %v, want 0", rl2.windowOffset)
	}
}

// noOffsetCounter is a minimal custom LimitCounter, standing in for a distributed
// backend (e.g. Redis), to confirm the offset applies only to the local counter.
type noOffsetCounter struct{}

func (noOffsetCounter) Config(int, time.Duration)                {}
func (noOffsetCounter) Increment(string, time.Time) error        { return nil }
func (noOffsetCounter) IncrementBy(string, time.Time, int) error { return nil }
func (noOffsetCounter) Get(string, time.Time, time.Time) (int, int, error) {
	return 0, 0, nil
}
