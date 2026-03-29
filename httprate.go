package httprate

import (
	"net"
	"net/http"
	"strings"
	"time"
)

// Limit creates a new [net/http] middleware that limits requests by the given
// request limit and window length. The returned middleware will call the next
// handler if the request limit is not exceeded.
func Limit(requestLimit int, windowLength time.Duration, options ...Option) func(next http.Handler) http.Handler {
	return NewRateLimiter(requestLimit, windowLength, options...).Handler
}

// KeyFunc is a function that derives a key for the given request.
type KeyFunc func(r *http.Request) (string, error)

// Option is a function that configures the rate limiter.
type Option func(rl *RateLimiter)

// ResponseHeaders defines custom response headers. If empty, the header is omitted.
type ResponseHeaders struct {
	// Limit is the total number of requests that are permitted before the rate limit
	// is exceeded. Default: "X-RateLimit-Limit".
	Limit string
	// Remaining is the number of requests remaining before the rate limit is
	// exceeded. Default: "X-RateLimit-Remaining".
	Remaining string
	// Increment is the number of requests incremented by the rate limiter. Default:
	// "X-RateLimit-Increment".
	Increment string
	// Reset is the time at which the rate limit will be reset. Default:
	// "X-RateLimit-Reset".
	Reset string
	// RetryAfter is the time in seconds after which the rate limit will be reset.
	// Default: "Retry-After".
	RetryAfter string
}

// LimitAll is a shortcut for [Limit] which uses a shared default key, resulting in
// a single rate-limiter for all requests.
func LimitAll(requestLimit int, windowLength time.Duration) func(next http.Handler) http.Handler {
	return Limit(requestLimit, windowLength)
}

// LimitByIP is a shortcut for [Limit] with the key function set to [KeyByIP],
// returning a new [net/http] middleware that limits requests by IP address.
func LimitByIP(requestLimit int, windowLength time.Duration) func(next http.Handler) http.Handler {
	return Limit(requestLimit, windowLength, WithKeyFuncs(KeyByIP))
}

// LimitByRealIP is a shortcut for [Limit] with the key function set to [KeyByRealIP],
// returning a new [net/http] middleware that limits requests by real IP address.
func LimitByRealIP(requestLimit int, windowLength time.Duration) func(next http.Handler) http.Handler {
	return Limit(requestLimit, windowLength, WithKeyFuncs(KeyByRealIP))
}

// LimitByEndpoint is a shortcut for [Limit] with the key function set to [KeyByEndpoint],
// returning a new [net/http] middleware that limits requests by endpoint.
func LimitByEndpoint(requestLimit int, windowLength time.Duration) func(next http.Handler) http.Handler {
	return Limit(requestLimit, windowLength, WithKeyFuncs(KeyByEndpoint))
}

// Key returns a key function that always returns the specified key.
func Key(key string) func(r *http.Request) (string, error) {
	return func(r *http.Request) (string, error) {
		return key, nil
	}
}

// KeyByIP uses the canonicalized remote address, [net/http.Request.RemoteAddr],
// to get the IP address.
func KeyByIP(r *http.Request) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return canonicalizeIP(ip), nil
}

// KeyByRealIP uses the "True-Client-IP", "X-Real-IP", and "X-Forwarded-For"
// headers (in that order of precedence) to get the IP address, after canonicalizing.
// If none of the headers are present, the remote address is used.
func KeyByRealIP(r *http.Request) (string, error) {
	var ip string

	if tcip := r.Header.Get("True-Client-IP"); tcip != "" {
		ip = tcip
	} else if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		ip = xrip
	} else if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		i := strings.Index(xff, ", ")
		if i == -1 {
			i = len(xff)
		}
		ip = xff[:i]
	} else {
		var err error
		ip, _, err = net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
	}

	return canonicalizeIP(ip), nil
}

// KeyByEndpoint uses the URL path, [net/url.URL.Path] as the key.
func KeyByEndpoint(r *http.Request) (string, error) {
	return r.URL.Path, nil
}

// WithKeyFuncs composes multiple key functions into a single key.
func WithKeyFuncs(keyFuncs ...KeyFunc) Option {
	return func(rl *RateLimiter) {
		if len(keyFuncs) > 0 {
			rl.keyFn = composedKeyFunc(keyFuncs...)
		}
	}
}

// WithKeyByIP is an option which sets the key function to [KeyByIP].
func WithKeyByIP() Option {
	return WithKeyFuncs(KeyByIP)
}

// WithKeyByRealIP is an option which sets the key function to [KeyByRealIP].
func WithKeyByRealIP() Option {
	return WithKeyFuncs(KeyByRealIP)
}

// WithKeyByEndpoint is an option which sets the key function to [KeyByEndpoint].
func WithKeyByEndpoint() Option {
	return WithKeyFuncs(KeyByEndpoint)
}

// WithLimitHandler is an option which sets the limit handler to the given
// [http.HandlerFunc]. If not set, the default limit handler is used.
func WithLimitHandler(h http.HandlerFunc) Option {
	return func(rl *RateLimiter) {
		rl.onRateLimited = h
	}
}

// WithErrorHandler is an option which sets the error handler to the given
// function. If not set, the default error handler is used.
func WithErrorHandler(h func(http.ResponseWriter, *http.Request, error)) Option {
	return func(rl *RateLimiter) {
		rl.onError = h
	}
}

// WithLimitCounter is an option which sets the limit counter to the given
// [LimitCounter]. If not set, the default [LocalLimitCounter] is used.
func WithLimitCounter(c LimitCounter) Option {
	return func(rl *RateLimiter) {
		rl.limitCounter = c
	}
}

// WithResponseHeaders is an option which sets the response headers to the given
// [ResponseHeaders]. If not set, the default response headers are used.
func WithResponseHeaders(headers ResponseHeaders) Option {
	return func(rl *RateLimiter) {
		rl.headers = headers
	}
}

// WithNoop is an option which does nothing.
func WithNoop() Option {
	return func(rl *RateLimiter) {}
}

// Skip is a middleware that allows the rate limiter headers to be applied onto a
// request, without actually including the request in the rate limit. Use this for
// endpoints that can be used for checking the rate limit, without affecting the
// rate limit. NOTE: This MUST be loaded in your middleware stack before the rate
// limiter.
//
// Example:
//
//	rl := httprate.Limit(100, time.Minute)
//	r.With(rl).Get(...) // Will be rate limited.
//	r.With(httprate.Skip, rl).Get(...) // Will not be rate limited, but still sets appropriate headers.
func Skip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithIncrement(r.Context(), 0)))
	})
}

// composedKeyFunc composes multiple key functions into a single key.
func composedKeyFunc(keyFuncs ...KeyFunc) KeyFunc {
	return func(r *http.Request) (string, error) {
		var key strings.Builder
		for i := 0; i < len(keyFuncs); i++ {
			k, err := keyFuncs[i](r)
			if err != nil {
				return "", err
			}
			key.WriteString(k)
			key.WriteRune(':')
		}
		return key.String(), nil
	}
}

// canonicalizeIP returns a form of ip suitable for comparison to other IPs.
// For IPv4 addresses, this is simply the whole string.
// For IPv6 addresses, this is the /64 prefix.
func canonicalizeIP(ip string) string {
	isIPv6 := false
	// This is how net.ParseIP decides if an address is IPv6
	// https://cs.opensource.google/go/go/+/refs/tags/go1.17.7:src/net/ip.go;l=704
	for i := 0; !isIPv6 && i < len(ip); i++ {
		switch ip[i] {
		case '.':
			// IPv4
			return ip
		case ':':
			// IPv6
			isIPv6 = true
		}
	}
	if !isIPv6 {
		// Not an IP address at all.
		return ip
	}

	ipv6 := net.ParseIP(ip)
	if ipv6 == nil {
		return ip
	}

	return ipv6.Mask(net.CIDRMask(64, 128)).String()
}
