package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

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
			time.Minute,
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
			w.Write([]byte("10 req/min\n"))
		})
	})

	r.Group(func(r chi.Router) {
		// Here we set another rate limit (3 req/min) for a group of handlers.
		//
		// Note: in practice you don't need to have so many layered rate-limiters,
		// but the example here is to illustrate how to control the machinery.
		r.Use(httprate.LimitByIP(3, time.Minute))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("3 req/min\n"))
		})
	})

	log.Printf("Serving at localhost:3333")
	log.Println()
	log.Printf("Try running:")
	log.Printf("curl -v http://localhost:3333")
	log.Printf("curl -v http://localhost:3333/admin")

	http.ListenAndServe(":3333", r)
}
