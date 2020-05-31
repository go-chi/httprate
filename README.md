# httprate

![](https://github.com/go-chi/stampede/workflows/build/badge.svg?branch=master)

net/http request rate limiter.


## Example

```go
package main

import (
  "net/http"

  "github.com/go-chi/chi"
  "github.com/go-chi/chi/middleware"
  "github.com/go-chi/httprate"
)

func main() {
  r := chi.NewRouter()
  r.Use(middleware.Logger)

  // Enable httprate request limiter to 2 respects per second with burst of 2.
  //
  // In the code example below, rate-limiting is bound to the request IP address
  // via the LimitByIP middleware handler.
  //
  // To have a single rate-limiter for all requests, use httprate.LimitAll(r, b).
  //
  // To rate-limit by a custom client key, use httprate.Limit(r, b, fn). For ex,
  // if you'd like to rate-limit by an authentication token, you can do:
  //
  // httprate.Limit(r, b, func(r *http.Request) (string, error) {
  //   k, _ := r.Context().Value("jwtToken").(string)
  //   return k, nil
  // })
  //
  r.Use(httprate.LimitByIP(2, 2))

  r.Get("/", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("."))
  })

  http.ListenAndServe(":3333", r)
}
```

## LICENSE

MIT
