# httprate - HTTP Rate Limiter

![CI workflow](https://github.com/go-chi/httprate/actions/workflows/ci.yml/badge.svg)
![Benchmark workflow](https://github.com/go-chi/httprate/actions/workflows/benchmark.yml/badge.svg)
[![GoDoc Widget]][GoDoc]

[GoDoc]: https://pkg.go.dev/github.com/go-chi/httprate
[GoDoc Widget]: https://godoc.org/github.com/go-chi/httprate?status.svg

`net/http` request rate limiter based on the Sliding Window Counter pattern inspired by
CloudFlare https://blog.cloudflare.com/counting-things-a-lot-of-different-things.

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

	// Enable httprate request limiter of 100 requests per minute.
	//
	// In the code example below, rate-limiting is bound to the request IP address
	// via the LimitByIP middleware handler.
	//
	// To have a single rate-limiter for all requests, use httprate.LimitAll(..).
	//
	// Please see _example/main.go for other more, or read the library code.
	r.Use(httprate.LimitByIP(100, time.Minute))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("."))
	})

	http.ListenAndServe(":3333", r)
}
```

## Common use cases

### Rate limit by IP and URL path (aka endpoint)
```go
r.Use(httprate.Limit(
	10,             // requests
	10*time.Second, // per duration
	httprate.WithKeyFuncs(httprate.KeyByIP, httprate.KeyByEndpoint),
))
```

### Rate limit by arbitrary keys
```go
r.Use(httprate.Limit(
	100,
	time.Minute,
	// an oversimplified example of rate limiting by a custom header
	httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
		return r.Header.Get("X-Access-Token"), nil
	}),
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
r.Use(httprate.Limit(
	10,
	time.Minute,
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
r.Use(httprate.Limit(
	10,
	time.Minute,
	httprate.WithErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, fmt.Sprintf(`{"error": %q}`, err), http.StatusPreconditionRequired)
	}),
	httprate.WithLimitCounter(customBackend),
))
```

### Send custom response headers

```go
r.Use(httprate.Limit(
	1000,
	time.Minute,
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
r.Use(httprate.Limit(
	1000,
	time.Minute,
	httprate.WithResponseHeaders(httprate.ResponseHeaders{}),
))
```

## LICENSE

MIT
