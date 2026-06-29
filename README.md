# httprate - HTTP Rate Limiter

![CI workflow](https://github.com/go-chi/httprate/actions/workflows/ci.yml/badge.svg)
![Benchmark workflow](https://github.com/go-chi/httprate/actions/workflows/benchmark.yml/badge.svg)
[![GoDoc Widget]][GoDoc]

[GoDoc]: https://pkg.go.dev/github.com/go-chi/httprate
[GoDoc Widget]: https://godoc.org/github.com/go-chi/httprate?status.svg

`net/http` request rate limiter based on the Sliding Window Counter pattern inspired by
CloudFlare https://blog.cloudflare.com/counting-things-a-lot-of-different-things.

> [!WARNING]
> **Security: `LimitByRealIP` / `KeyByRealIP` / `WithKeyByRealIP` are deprecated.**
> They derive the rate-limit key from client-supplied headers (`True-Client-IP`,
> `X-Real-IP`, `X-Forwarded-For`) with no proxy-trust check, so a remote
> attacker can spoof the key — either rotating it to **evade** the limit or
> pinning it to a victim's IP to **lock that victim out** (HTTP 429). This is the
> same flaw fixed in chi's `middleware.RealIP` (see
> [GHSA-9g5q-2w5x-hmxf](https://github.com/go-chi/chi/security/advisories/GHSA-9g5q-2w5x-hmxf),
> [GHSA-rjr7-jggh-pgcp](https://github.com/go-chi/chi/security/advisories/GHSA-rjr7-jggh-pgcp),
> [GHSA-3fxj-6jh8-hvhx](https://github.com/go-chi/chi/security/advisories/GHSA-3fxj-6jh8-hvhx)).
> Rate-limit by a **trusted** client IP instead — see
> [Rate limit by client IP behind a proxy](#rate-limit-by-client-ip-behind-a-proxy).

The sliding window counter pattern is accurate, smooths traffic and offers a simple counter
design to share a rate-limit among a cluster of servers. For example, if you'd like
to use redis to coordinate a rate-limit across a group of microservices you just need
to implement the `httprate.LimitCounter` interface to support an atomic increment and get.

## Backends

- [x] Local in-memory backend (default)
- [x] Redis backend: https://github.com/go-chi/httprate-redis

## Example

```go
package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	// Enable httprate request limiter of 100 requests per minute, keyed by the
	// client IP.
	//
	// There is no safe default IP source, so you state your trust model
	// explicitly: one of chi's middleware.ClientIPFrom* middlewares (chi v5.3.0+)
	// resolves the client IP into the request context, and
	// KeyFromContext(middleware.GetClientIP) reads it. Pick the chi
	// ClientIPFrom* that matches your deployment — here we assume
	// the server is directly exposed to clients (no proxy), so the client IP is
	// the TCP peer (RemoteAddr). Behind a reverse proxy or CDN, use
	// ClientIPFromXFF / ClientIPFromHeader instead; see "Rate limit by client IP
	// behind a proxy" below.
	//
	// To have a single rate-limiter for all requests, use a constant key:
	// httprate.LimitBy(.., httprate.Key("*")).
	//
	// NOTE: middleware.GetClientIP returns the full client IP. To stop IPv6
	// clients from rotating within their /64 to win fresh buckets, canonicalize
	// the IP to /64 — see "Rate limit by client IP behind a proxy" below and
	// _example/main.go.
	//
	// Please see _example/main.go for more, or read the library code.
	r.Use(middleware.ClientIPFromRemoteAddr)
	r.Use(httprate.LimitBy(100, time.Minute, httprate.KeyFromContext(middleware.GetClientIP)))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("."))
	})

	http.ListenAndServe(":3333", r)
}
```

## Common use cases

### Rate limit by client IP behind a proxy

If your app runs behind a reverse proxy, load balancer, or CDN, the request's
`RemoteAddr` is the proxy, not the client. Resolving the real client IP safely
means deciding *which* hop to trust — there is no safe default, and trusting a
client-supplied header blindly is exactly the spoofing bug behind the deprecated
`LimitByRealIP`.

Use chi's [`middleware.ClientIPFrom*`](https://pkg.go.dev/github.com/go-chi/chi/v5/middleware#ClientIPFromXFF)
middlewares (chi `v5.3.0+`) to resolve a trusted client IP, then rate-limit by
it with `LimitBy` + `KeyFromContext`:

```go
import (
	"context"
	"net/netip"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

// 1. Resolve a trusted client IP. Pick exactly ONE that matches your
//    deployment (see the table below):
r.Use(middleware.ClientIPFromXFF("10.0.0.0/8"))

// 2. Rate-limit by that trusted client IP.
r.Use(httprate.LimitBy(100, time.Minute,
	httprate.KeyFromContext(clientIPKey),
))

// clientIPKey reads the trusted client IP resolved in step 1 and canonicalizes
// it for rate-limiting. chi's middleware.GetClientIPAddr returns the *full*
// client IP as a net/netip.Addr; we bucket IPv6 clients by their /64 prefix so
// a client can't rotate within its own /64 (2^64 addresses via SLAAC) to win a
// fresh bucket on every request. IPv4 is used as-is.
func clientIPKey(ctx context.Context) string {
	ip := middleware.GetClientIPAddr(ctx)
	if !ip.IsValid() {
		return "" // no ClientIPFrom* upstream — see the note below
	}
	if ip.Is4() {
		return ip.String()
	}
	return netip.PrefixFrom(ip, 64).Masked().Addr().String() // IPv6 → /64
}
```

> [!NOTE]
> Rate-limiting by the full IPv6 address (`KeyFromContext(middleware.GetClientIP)`)
> lets an IPv6 client rotate within its own `/64` to get a fresh bucket per
> request and bypass the limit. The `clientIPKey` helper above buckets IPv6 by
> `/64` — adjust the prefix (e.g. `/56`, `/48`) if your clients are delegated a
> larger block. The deprecated `KeyByIP` / `KeyByRealIP` did this `/64`
> canonicalization for you; the explicit helper keeps it while making the trust
> model and prefix your choice.

Pick the one `ClientIPFrom*` middleware that matches how requests reach you:

| Your setup | Use |
| --- | --- |
| Directly on the public internet, no proxy | `middleware.ClientIPFromRemoteAddr` |
| Behind nginx (`X-Real-IP`), Cloudflare (`CF-Connecting-IP`), Apache (`X-Client-IP`) | `middleware.ClientIPFromHeader("X-Real-IP")` |
| Behind one or more proxies whose IP ranges you can list | `middleware.ClientIPFromXFF("10.0.0.0/8", ...)` |
| Behind a known, fixed number of proxies with dynamic IPs | `middleware.ClientIPFromXFFTrustedProxies(2)` |

See chi's [Choosing a ClientIP middleware](https://pkg.go.dev/github.com/go-chi/chi/v5/middleware#hdr-Choosing_a_ClientIP_middleware)
for the full picker.

> [!IMPORTANT]
> If no `ClientIPFrom*` middleware is installed upstream, `middleware.GetClientIP`
> returns `""` and **every request shares a single global rate-limit bucket**.
> That's strictly more restrictive (not a security hole), but it's a footgun —
> you'll trip the limit after `requestLimit` total requests in dev. Make sure
> exactly one `ClientIPFrom*` middleware runs before the limiter.

`KeyFromContext` is generic over `func(context.Context) string`, so it isn't
tied to chi — pass your own extractor for echo, fiber, gin, or custom
middleware that stashes the client IP (or tenant/user ID) in the request
context.

### Rate limit by IP and URL path (aka endpoint)
```go
r.Use(httprate.LimitBy(
	10,             // requests
	10*time.Second, // per duration
	httprate.JoinKeys(httprate.KeyFromContext(middleware.GetClientIP), httprate.KeyByEndpoint),
))
```

### Rate limit by arbitrary keys
```go
r.Use(httprate.LimitBy(
	100,
	time.Minute,
	// an oversimplified example of rate limiting by a custom header
	func(r *http.Request) (string, error) {
		return r.Header.Get("X-Access-Token"), nil
	},
))
```

### Rate limit by request payload
```go
// Rate-limiter for login endpoint.
loginRateLimiter := httprate.NewRateLimiter(5, time.Minute)

r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil || payload.Username == "" || payload.Password == "" {
		w.WriteHeader(400)
		return
	}

	// Rate-limit login at 5 req/min.
	if loginRateLimiter.RespondOnLimit(w, r, payload.Username) {
		return
	}

	w.Write([]byte("login at 5 req/min\n"))
})
```

### Send specific response for rate-limited requests

The default response is `HTTP 429` with `Too Many Requests` body. You can override it with:

```go
r.Use(httprate.LimitBy(
	10,
	time.Minute,
	httprate.KeyFromContext(middleware.GetClientIP),
	httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": "Rate-limited. Please, slow down."}`, http.StatusTooManyRequests)
	}),
))
```

### Send specific response on errors

An error can be returned by:
- A custom key function provided by `httprate.WithKeyFunc(customKeyFn)`
- A custom backend provided by `httprateredis.WithRedisLimitCounter(customBackend)`
    - The default local in-memory counter is guaranteed not return any errors
    - Backends that fall-back to the local in-memory counter (e.g. [httprate-redis](https://github.com/go-chi/httprate-redis)) can choose not to return any errors either

```go
r.Use(httprate.LimitBy(
	10,
	time.Minute,
	httprate.KeyFromContext(middleware.GetClientIP),
	httprate.WithErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, fmt.Sprintf(`{"error": %q}`, err), http.StatusPreconditionRequired)
	}),
	httprate.WithLimitCounter(customBackend),
))
```

### Send custom response headers

```go
r.Use(httprate.LimitBy(
	1000,
	time.Minute,
	httprate.KeyFromContext(middleware.GetClientIP),
	httprate.WithResponseHeaders(httprate.ResponseHeaders{
		Limit:      "X-RateLimit-Limit",
		Remaining:  "X-RateLimit-Remaining",
		Reset:      "X-RateLimit-Reset",
		RetryAfter: "Retry-After",
		Increment:  "", // omit
	}),
))
```

### Omit response headers

```go
r.Use(httprate.LimitBy(
	1000,
	time.Minute,
	httprate.KeyFromContext(middleware.GetClientIP),
	httprate.WithResponseHeaders(httprate.ResponseHeaders{}),
))
```

## LICENSE

MIT
