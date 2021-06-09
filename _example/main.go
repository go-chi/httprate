package main

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/httprate"
)

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	// Overall rate-limiter, keyed by IP and URL path (aka endpoint).
	//
	// This means each user (by IP) will receive a unique limit counter per endpoint.
	// r.Use(httprate.Limit(10, 10*time.Second, httprate.WithKeyFuncs(httprate.KeyByIP, httprate.KeyByEndpoint)))

	r.Route("/admin", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Note: this is a mock middleware to set a userID on the request context
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "userID", "123")))
			})
		})

		// Here we set a specific rate limit by ip address and userID
		r.Use(httprate.Limit(
			10,
			10*time.Second,
			httprate.WithKeyFuncs(httprate.KeyByIP, func(r *http.Request) (string, error) {
				token := r.Context().Value("userID").(string)
				return token, nil
			}),
			httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
				// We can send custom responses for the rate limited requests, e.g. a JSON message
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error": "Too many requests"}`))
			}),
		))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("admin."))
		})
	})

	r.Group(func(r chi.Router) {
		// Here we set another rate limit for a group of handlers.
		//
		// Note: in practice you don't need to have so many layered rate-limiters,
		// but the example here is to illustrate how to control the machinery.
		r.Use(httprate.LimitByIP(3, 5*time.Second))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("."))
		})
	})

	http.ListenAndServe(":3333", r)
}
