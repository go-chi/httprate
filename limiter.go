package httprate

import (
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

type LimitCounter interface {
	Config(requestLimit int, windowLength time.Duration)
	Increment(key string, currentWindow time.Time) error
	IncrementBy(key string, currentWindow time.Time, amount int) error
	Get(key string, currentWindow, previousWindow time.Time) (int, int, error)
}

func NewRateLimiter(requestLimit int, windowLength time.Duration, options ...Option) *rateLimiter {
	return newRateLimiter(requestLimit, windowLength, options...)
}

func newRateLimiter(requestLimit int, windowLength time.Duration, options ...Option) *rateLimiter {
	rl := &rateLimiter{
		requestLimit: requestLimit,
		windowLength: windowLength,
	}

	for _, opt := range options {
		opt(rl)
	}

	if rl.keyFn == nil {
		rl.keyFn = func(r *http.Request) (string, error) {
			return "*", nil
		}
	}

	if rl.limitCounter == nil {
		rl.limitCounter = &localCounter{
			counters:     make(map[uint64]*count),
			windowLength: windowLength,
		}
	}
	rl.limitCounter.Config(requestLimit, windowLength)

	if rl.onRequestLimit == nil {
		rl.onRequestLimit = func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		}
	}

	return rl
}

type rateLimiter struct {
	requestLimit   int
	windowLength   time.Duration
	keyFn          KeyFunc
	limitCounter   LimitCounter
	onRequestLimit http.HandlerFunc
	mu             sync.Mutex
}

func (l *rateLimiter) Counter() LimitCounter {
	return l.limitCounter
}

func (l *rateLimiter) Status(key string) (bool, float64, error) {
	return l.calculateRate(key, l.requestLimit)
}

func (l *rateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, err := l.keyFn(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusPreconditionRequired)
			return
		}

		currentWindow := time.Now().UTC().Truncate(l.windowLength)
		ctx := r.Context()

		limit := l.requestLimit
		if val := getRequestLimit(ctx); val > 0 {
			limit = val
		}
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", 0))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", currentWindow.Add(l.windowLength).Unix()))

		l.mu.Lock()
		_, rateFloat, err := l.calculateRate(key, limit)
		if err != nil {
			l.mu.Unlock()
			http.Error(w, err.Error(), http.StatusPreconditionRequired)
			return
		}
		rate := int(math.Round(rateFloat))

		increment := getIncrement(r.Context())
		if increment > 1 {
			w.Header().Set("X-RateLimit-Increment", fmt.Sprintf("%d", increment))
		}

		if rate+increment > limit {
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", limit-rate))

			l.mu.Unlock()
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(l.windowLength.Seconds()))) // RFC 6585
			l.onRequestLimit(w, r)
			return
		}

		err = l.limitCounter.IncrementBy(key, currentWindow, increment)
		if err != nil {
			l.mu.Unlock()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		l.mu.Unlock()

		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", limit-rate-increment))

		next.ServeHTTP(w, r)
	})
}

func (l *rateLimiter) calculateRate(key string, requestLimit int) (bool, float64, error) {
	t := time.Now().UTC()
	currentWindow := t.Truncate(l.windowLength)
	previousWindow := currentWindow.Add(-l.windowLength)

	currCount, prevCount, err := l.limitCounter.Get(key, currentWindow, previousWindow)
	if err != nil {
		return false, 0, err
	}

	diff := t.Sub(currentWindow)
	rate := float64(prevCount)*(float64(l.windowLength)-float64(diff))/float64(l.windowLength) + float64(currCount)
	if rate > float64(requestLimit) {
		return false, rate, nil
	}

	return true, rate, nil
}

type localCounter struct {
	counters     map[uint64]*count
	windowLength time.Duration
	lastEvict    time.Time
	mu           sync.Mutex
}

var _ LimitCounter = &localCounter{}

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
