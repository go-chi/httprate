package httprate

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// LimitBy is the canonical entry point for rate-limiting by an explicit key.
//
// It is shorthand for Limit with keyFn installed as the rate-limit key. The
// key is a required positional argument, so every call site has to state, on
// purpose, what it rate-limits by.
//
// To rate-limit by client IP behind a proxy, pair it with one of chi's
// middleware.ClientIPFrom* middlewares (chi v5.3.0+) and KeyFromContext:
//
//	r.Use(middleware.ClientIPFromXFF("10.0.0.0/8"))
//	r.Use(httprate.LimitBy(100, time.Minute, httprate.KeyFromContext(middleware.GetClientIP)))
//
// Use JoinKeys to rate-limit by more than one dimension at once:
//
//	r.Use(httprate.LimitBy(100, time.Minute,
//		httprate.JoinKeys(httprate.KeyByIP, httprate.KeyByEndpoint)))
func LimitBy(requestLimit int, windowLength time.Duration, keyFn KeyFunc, options ...Option) func(next http.Handler) http.Handler {
	return NewRateLimiter(requestLimit, windowLength, append([]Option{WithKeyFuncs(keyFn)}, options...)...).Handler
}

type KeyFunc func(r *http.Request) (string, error)
type Option func(rl *RateLimiter)

// Set custom response headers. If empty, the header is omitted.
type ResponseHeaders struct {
	Limit      string // Default: X-RateLimit-Limit
	Remaining  string // Default: X-RateLimit-Remaining
	Increment  string // Default: X-RateLimit-Increment
	Reset      string // Default: X-RateLimit-Reset
	RetryAfter string // Default: Retry-After
}

func Key(key string) func(r *http.Request) (string, error) {
	return func(r *http.Request) (string, error) {
		return key, nil
	}
}

// KeyFromContext builds a KeyFunc that reads the rate-limit key from the
// request context using the given extractor. It is the safe, router-agnostic
// way to rate-limit by a trusted client IP: pair it with chi's
// middleware.GetClientIP (chi v5.3.0+), which returns the IP resolved by
// whichever middleware.ClientIPFrom* middleware you installed upstream.
//
//	r.Use(middleware.ClientIPFromXFF("10.0.0.0/8"))
//	r.Use(httprate.LimitBy(100, time.Minute, httprate.KeyFromContext(middleware.GetClientIP)))
//
// The extractor is generic over func(context.Context) string, so it is not
// tied to chi — any framework or custom middleware that stashes a value in the
// request context (tenant ID, API key, user ID, ...) works the same way.
//
// WARNING: if the extractor returns an empty string, every request shares a
// single rate-limit bucket and the limit fires after requestLimit requests
// system-wide. When using chi's middleware.GetClientIP, make sure exactly one
// of middleware.ClientIPFromHeader, middleware.ClientIPFromXFF,
// middleware.ClientIPFromXFFTrustedProxies, or middleware.ClientIPFromRemoteAddr
// is installed upstream; otherwise GetClientIP returns "" for every request.
func KeyFromContext(extractor func(ctx context.Context) string) KeyFunc {
	return func(r *http.Request) (string, error) {
		return extractor(r.Context()), nil
	}
}

func KeyByEndpoint(r *http.Request) (string, error) {
	return r.URL.Path, nil
}

func WithKeyFuncs(keyFuncs ...KeyFunc) Option {
	return func(rl *RateLimiter) {
		if len(keyFuncs) > 0 {
			rl.keyFn = JoinKeys(keyFuncs...)
		}
	}
}

func WithLimitHandler(h http.HandlerFunc) Option {
	return func(rl *RateLimiter) {
		rl.onRateLimited = h
	}
}

func WithErrorHandler(h func(http.ResponseWriter, *http.Request, error)) Option {
	return func(rl *RateLimiter) {
		rl.onError = h
	}
}

func WithLimitCounter(c LimitCounter) Option {
	return func(rl *RateLimiter) {
		rl.limitCounter = c
	}
}

func WithResponseHeaders(headers ResponseHeaders) Option {
	return func(rl *RateLimiter) {
		rl.headers = headers
	}
}

func WithNoop() Option {
	return func(rl *RateLimiter) {}
}

// JoinKeys joins the results of several KeyFuncs into a single key with ":"
// separators, so they can be passed to LimitBy's positional key slot for
// multi-dimensional rate-limiting:
//
//	r.Use(httprate.LimitBy(100, time.Minute,
//		httprate.JoinKeys(httprate.KeyByIP, httprate.KeyByEndpoint)))
//
// It is the positional-argument equivalent of WithKeyFuncs. If any component
// KeyFunc returns an error, the joined key returns that error.
func JoinKeys(fns ...KeyFunc) KeyFunc {
	return func(r *http.Request) (string, error) {
		var key strings.Builder
		for i := 0; i < len(fns); i++ {
			k, err := fns[i](r)
			if err != nil {
				return "", err
			}
			key.WriteString(k)
			key.WriteRune(':')
		}
		return key.String(), nil
	}
}
