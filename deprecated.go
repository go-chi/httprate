package httprate

import (
	"net"
	"net/http"
	"strings"
	"time"
)

// This file collects the deprecated public surface in one place. These are kept
// for backward compatibility and still compile/behave as before, but new code
// should migrate to the replacements called out in each doc comment. httprate
// has not reached a stable v1, so a future major release may remove them.

// Deprecated: Use LimitBy(requestLimit, windowLength, keyFn, options...) instead,
// which makes the rate-limit key an explicit, required argument rather than an
// optional WithKeyFuncs. Pass the key function directly (e.g. httprate.KeyByIP),
// or httprate.Key("*") for a single global bucket. The remaining options
// (WithLimitCounter, WithLimitHandler, WithResponseHeaders, ...) carry over
// unchanged as LimitBy's trailing variadic.
func Limit(requestLimit int, windowLength time.Duration, options ...Option) func(next http.Handler) http.Handler {
	// Key("*") is the default key; any WithKeyFuncs in options overrides it.
	return LimitBy(requestLimit, windowLength, Key("*"), options...)
}

// Deprecated: Use LimitBy(requestLimit, windowLength, Key("*")) instead — a
// single global rate-limit bucket keyed by a constant. (LimitAll already keys
// every request by "*" under the hood; this just makes that explicit.)
func LimitAll(requestLimit int, windowLength time.Duration) func(next http.Handler) http.Handler {
	return LimitBy(requestLimit, windowLength, Key("*"))
}

// Deprecated: Use LimitBy(requestLimit, windowLength, KeyByIP) instead. This is
// an ergonomic rename with identical behavior, not a security fix.
func LimitByIP(requestLimit int, windowLength time.Duration) func(next http.Handler) http.Handler {
	return LimitBy(requestLimit, windowLength, KeyByIP)
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
	return LimitBy(requestLimit, windowLength, KeyByRealIP)
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

// Deprecated: WithKeyByRealIP installs the spoofable KeyByRealIP and lets a
// remote attacker forge the rate-limit key — see GHSA-9g5q-2w5x-hmxf,
// GHSA-rjr7-jggh-pgcp, GHSA-3fxj-6jh8-hvhx for the equivalent flaw in chi's
// middleware.RealIP. Install one of chi's middleware.ClientIPFrom* middlewares
// (chi v5.3.0+) and use LimitBy with KeyFromContext(middleware.GetClientIP)
// instead.
func WithKeyByRealIP() Option {
	return WithKeyFuncs(KeyByRealIP)
}
