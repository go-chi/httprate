package httprate

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"
)

func Limit(requestLimit int, windowLength time.Duration, options ...Option) func(next http.Handler) http.Handler {
	return NewRateLimiter(requestLimit, windowLength, options...).Handler
}

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
// Use ComposeKeys to rate-limit by more than one dimension at once:
//
//	r.Use(httprate.LimitBy(100, time.Minute,
//		httprate.ComposeKeys(httprate.KeyByIP, httprate.KeyByEndpoint)))
func LimitBy(requestLimit int, windowLength time.Duration, keyFn KeyFunc, options ...Option) func(next http.Handler) http.Handler {
	return Limit(requestLimit, windowLength, append([]Option{WithKeyFuncs(keyFn)}, options...)...)
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

func LimitAll(requestLimit int, windowLength time.Duration) func(next http.Handler) http.Handler {
	return Limit(requestLimit, windowLength)
}

// Deprecated: Use LimitBy(requestLimit, windowLength, KeyByIP) instead. This is
// an ergonomic rename with identical behavior, not a security fix.
func LimitByIP(requestLimit int, windowLength time.Duration) func(next http.Handler) http.Handler {
	return Limit(requestLimit, windowLength, WithKeyFuncs(KeyByIP))
}

// Deprecated: LimitByRealIP is built on the spoofable KeyByRealIP and lets a
// remote attacker forge the rate-limit key — see GHSA-9g5q-2w5x-hmxf,
// GHSA-rjr7-jggh-pgcp, GHSA-3fxj-6jh8-hvhx for the equivalent flaw in chi's
// middleware.RealIP. Install one of chi's middleware.ClientIPFrom* middlewares
// (chi v5.3.0+) and use LimitBy with KeyFromContext instead:
//
//	r.Use(middleware.ClientIPFromXFF("10.0.0.0/8"))
//	r.Use(httprate.LimitBy(requestLimit, windowLength, httprate.KeyFromContext(middleware.GetClientIP)))
func LimitByRealIP(requestLimit int, windowLength time.Duration) func(next http.Handler) http.Handler {
	return Limit(requestLimit, windowLength, WithKeyFuncs(KeyByRealIP))
}

func Key(key string) func(r *http.Request) (string, error) {
	return func(r *http.Request) (string, error) {
		return key, nil
	}
}

func KeyByIP(r *http.Request) (string, error) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return canonicalizeIP(ip), nil
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

// Deprecated: KeyByRealIP trusts the client-supplied True-Client-IP,
// X-Real-IP, and X-Forwarded-For headers without verifying any proxy chain, so
// a remote attacker can forge the rate-limit key — see GHSA-9g5q-2w5x-hmxf,
// GHSA-rjr7-jggh-pgcp, GHSA-3fxj-6jh8-hvhx for the equivalent flaw in chi's
// middleware.RealIP. On a rate-limiter this is two-sided: an attacker can evade
// the limit by rotating the spoofed header (unbounded buckets) or lock a victim
// out by pinning the header to the victim's IP (exhausting their bucket).
//
// Install one of chi's middleware.ClientIPFrom* middlewares (chi v5.3.0+) and
// use KeyFromContext(middleware.GetClientIP) instead:
//
//	r.Use(middleware.ClientIPFromXFF("10.0.0.0/8"))
//	r.Use(httprate.LimitBy(100, time.Minute, httprate.KeyFromContext(middleware.GetClientIP)))
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

func KeyByEndpoint(r *http.Request) (string, error) {
	return r.URL.Path, nil
}

func WithKeyFuncs(keyFuncs ...KeyFunc) Option {
	return func(rl *RateLimiter) {
		if len(keyFuncs) > 0 {
			rl.keyFn = composedKeyFunc(keyFuncs...)
		}
	}
}

func WithKeyByIP() Option {
	return WithKeyFuncs(KeyByIP)
}

// Deprecated: WithKeyByRealIP installs the spoofable KeyByRealIP and lets a
// remote attacker forge the rate-limit key — see GHSA-9g5q-2w5x-hmxf,
// GHSA-rjr7-jggh-pgcp, GHSA-3fxj-6jh8-hvhx for the equivalent flaw in chi's
// middleware.RealIP. Install one of chi's middleware.ClientIPFrom* middlewares
// (chi v5.3.0+) and use LimitBy with KeyFromContext(middleware.GetClientIP)
// instead.
func WithKeyByRealIP() Option {
	return WithKeyFuncs(KeyByRealIP)
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

// ComposeKeys concatenates the results of several KeyFuncs into a single key,
// joined with ":" separators, so they can be passed to LimitBy's positional
// key slot for multi-dimensional rate-limiting:
//
//	r.Use(httprate.LimitBy(100, time.Minute,
//		httprate.ComposeKeys(httprate.KeyByIP, httprate.KeyByEndpoint)))
//
// It is the positional-argument equivalent of WithKeyFuncs. If any component
// KeyFunc returns an error, the composed key returns that error.
func ComposeKeys(fns ...KeyFunc) KeyFunc {
	return composedKeyFunc(fns...)
}

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
			break
		}
	}
	if !isIPv6 {
		// Not an IP address at all
		return ip
	}

	ipv6 := net.ParseIP(ip)
	if ipv6 == nil {
		return ip
	}

	return ipv6.Mask(net.CIDRMask(64, 128)).String()
}
