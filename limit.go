package httprate

import (
	"net"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// Limit allows requests up to rate r and permits bursts of at most b tokens for
// clientID string for incoming http request.
func Limit(rps, burst int, clientID ClientIDFunc) func(next http.Handler) http.Handler {
	rateLimiter := newRateLimiter(rps, burst)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k, err := clientID(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusPreconditionRequired)
				return
			}

			limiter := rateLimiter.getLimiter(k)
			if !limiter.Allow() {
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type ClientIDFunc func(r *http.Request) (string, error)

func LimitByIP(rps, burst int) func(next http.Handler) http.Handler {
	ipKeyFn := func(r *http.Request) (string, error) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		return ip, nil
	}
	return Limit(rps, burst, ipKeyFn)
}

func LimitAll(rps, burst int) func(next http.Handler) http.Handler {
	keyFn := func(r *http.Request) (string, error) {
		return "*", nil
	}
	return Limit(rps, burst, keyFn)
}

type rateLimiter struct {
	limiters map[string]*rate.Limiter
	r        int // limiter rate
	b        int // limiter bucket size
	sync.RWMutex
}

func newRateLimiter(r, b int) *rateLimiter {
	return &rateLimiter{
		limiters: make(map[string]*rate.Limiter),
		r:        r,
		b:        b,
	}
}

func (l *rateLimiter) getLimiter(key string) *rate.Limiter {
	l.RLock()
	limiter, ok := l.limiters[key]
	l.RUnlock()
	if ok {
		return limiter
	}
	l.Lock()
	if limiter, ok = l.limiters[key]; !ok {
		limiter = rate.NewLimiter(rate.Limit(l.r), l.b)
		l.limiters[key] = limiter
	}
	l.Unlock()
	return limiter
}
