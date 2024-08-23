package main

import (
	"context"
	"encoding/json"
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

	// Rate-limit all routes at 1000 req/min by IP address.
	r.Use(httprate.LimitByIP(1000, time.Minute))

	r.Route("/admin", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Note: This is a mock middleware to set a userID on the request context
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "userID", "123")))
			})
		})

		// Rate-limit admin routes at 10 req/s by userID.
		r.Use(httprate.Limit(
			10, time.Second,
			httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
				token, _ := r.Context().Value("userID").(string)
				return token, nil
			}),
		))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("admin at 10 req/s\n"))
		})
	})

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
		if loginRateLimiter.OnLimit(w, r, payload.Username) {
			return
		}

		w.Write([]byte("login at 5 req/min\n"))
	})

	log.Printf("Serving at localhost:3333")
	log.Println()
	log.Printf("Try running:")
	log.Printf(`curl -v http://localhost:3333?[0-1000]`)
	log.Printf(`curl -v http://localhost:3333/admin?[1-12]`)
	log.Printf(`curl -v http://localhost:3333/login\?[1-8] --data '{"username":"alice","password":"***"}'`)

	http.ListenAndServe(":3333", r)
}
