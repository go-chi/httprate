package httprate

import (
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"
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
		headers: ResponseHeaders{
			Limit:      "X-RateLimit-Limit",
			Remaining:  "X-RateLimit-Remaining",
			Increment:  "X-RateLimit-Increment",
			Reset:      "X-RateLimit-Reset",
			RetryAfter: "Retry-After",
		},
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
			latestWindow:     time.Now().UTC().Truncate(windowLength),
			latestCounters:   make(map[uint64]int),
			previousCounters: make(map[uint64]int),
			windowLength:     windowLength,
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
	headers        ResponseHeaders
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
		setHeader(w, l.headers.Limit, fmt.Sprintf("%d", limit))
		setHeader(w, l.headers.Reset, fmt.Sprintf("%d", currentWindow.Add(l.windowLength).Unix()))

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
			setHeader(w, l.headers.Increment, fmt.Sprintf("%d", increment))
		}

		if rate+increment > limit {
			setHeader(w, l.headers.Remaining, fmt.Sprintf("%d", limit-rate))

			l.mu.Unlock()
			setHeader(w, l.headers.RetryAfter, fmt.Sprintf("%d", int(l.windowLength.Seconds()))) // RFC 6585
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

		setHeader(w, l.headers.Remaining, fmt.Sprintf("%d", limit-rate-increment))

		next.ServeHTTP(w, r)
	})
}

func (l *rateLimiter) calculateRate(key string, requestLimit int) (bool, float64, error) {
	now := time.Now().UTC()
	currentWindow := now.Truncate(l.windowLength)
	previousWindow := currentWindow.Add(-l.windowLength)

	currCount, prevCount, err := l.limitCounter.Get(key, currentWindow, previousWindow)
	if err != nil {
		return false, 0, err
	}

	diff := now.Sub(currentWindow)
	rate := float64(prevCount)*(float64(l.windowLength)-float64(diff))/float64(l.windowLength) + float64(currCount)
	if rate > float64(requestLimit) {
		return false, rate, nil
	}

	return true, rate, nil
}

func setHeader(w http.ResponseWriter, key string, value string) {
	if key != "" {
		w.Header().Set(key, value)
	}
}
