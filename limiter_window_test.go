package httprate

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestOnLimitUsesSameWindowAfterWaitingForLock(t *testing.T) {
	for _, customCounter := range []bool{false, true} {
		name := "default counter"
		if customCounter {
			name = "custom counter"
		}
		t.Run(name, func(t *testing.T) {
			const windowLength = 200 * time.Millisecond
			var options []Option
			if customCounter {
				options = append(options, WithLimitCounter(NewLocalLimitCounter(windowLength)))
			}
			l := NewRateLimiter(10, windowLength, options...)
			counter := &windowRecordingCounter{LimitCounter: l.Counter()}
			l.limitCounter = counter

			// Hold the limiter lock as another request would while accessing a slow
			// counter. The queued request reaches its first header before the lock.
			l.mu.Lock()
			locked := true
			defer func() {
				if locked {
					l.mu.Unlock()
				}
			}()
			entered := make(chan time.Time, 1)
			w := &windowNotifyingWriter{
				ResponseRecorder: httptest.NewRecorder(),
				onHeader:         func() { entered <- l.currentWindow(time.Now().UTC()) },
			}
			done := make(chan bool, 1)
			go func() { done <- l.OnLimit(w, httptest.NewRequest("GET", "/", nil), "queued") }()

			var queuedWindow time.Time
			select {
			case queuedWindow = <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach the limiter")
			}
			time.Sleep(time.Until(queuedWindow.Add(windowLength)) + 20*time.Millisecond)
			// Simulate the request holding the lock recording another key in the
			// new window before releasing the lock to the queued request.
			currentWindow := l.currentWindow(time.Now().UTC())
			if err := counter.LimitCounter.Increment("other", currentWindow); err != nil {
				t.Fatal(err)
			}
			l.mu.Unlock()
			locked = false
			select {
			case limited := <-done:
				if limited {
					t.Fatal("first request for queued key was limited")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("request did not finish after releasing the lock")
			}

			if !counter.getWindow.After(queuedWindow) {
				t.Errorf("Get used stale window %v, queued in %v", counter.getWindow, queuedWindow)
			}
			if !counter.getWindow.Equal(counter.incrementWindow) {
				t.Errorf("Get window %v differs from IncrementBy window %v", counter.getWindow, counter.incrementWindow)
			}
			if got, want := w.Header().Get("X-RateLimit-Reset"), strconv.FormatInt(counter.getWindow.Add(windowLength).Unix(), 10); got != want {
				t.Errorf("reset header = %s, want %s", got, want)
			}
			curr, prev, err := counter.LimitCounter.Get("other", counter.getWindow, counter.getWindow.Add(-windowLength))
			// A heavily loaded scheduler can advance another window before the
			// queued request resumes; allow the normal shift/expiry in that case.
			wantCurr, wantPrev := 0, 0
			switch counter.getWindow.Sub(currentWindow) {
			case 0:
				wantCurr = 1
			case windowLength:
				wantPrev = 1
			}
			if err != nil || curr != wantCurr || prev != wantPrev {
				t.Errorf("other key's counts = (%d, %d, %v), want (%d, %d, nil)", curr, prev, err, wantCurr, wantPrev)
			}
		})
	}
}

type windowRecordingCounter struct {
	LimitCounter
	getWindow       time.Time
	incrementWindow time.Time
}

func (c *windowRecordingCounter) Get(key string, currentWindow, previousWindow time.Time) (int, int, error) {
	c.getWindow = currentWindow
	return c.LimitCounter.Get(key, currentWindow, previousWindow)
}

func (c *windowRecordingCounter) IncrementBy(key string, currentWindow time.Time, amount int) error {
	c.incrementWindow = currentWindow
	return c.LimitCounter.IncrementBy(key, currentWindow, amount)
}

type windowNotifyingWriter struct {
	*httptest.ResponseRecorder
	onHeader func()
	once     sync.Once
}

func (w *windowNotifyingWriter) Header() http.Header {
	w.once.Do(w.onHeader)
	return w.ResponseRecorder.Header()
}
