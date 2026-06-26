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

	// Rate-limit all routes at 1000 req/min by IP address (RemoteAddr, the TCP
	// peer). Use this when the server is directly exposed to clients.
	r.Use(httprate.LimitBy(1000, time.Minute, httprate.KeyByIP))

	// Rate-limit by the *trusted* client IP when running behind a reverse
	// proxy / CDN. middleware.ClientIPFromXFF resolves the client IP from the
	// X-Forwarded-For chain, skipping the trusted proxy CIDR(s), and stashes it
	// in the request context; KeyFromContext(middleware.GetClientIP) reads it.
	//
	// This is the safe replacement for the deprecated, spoofable
	// httprate.LimitByRealIP — a client can no longer forge X-Forwarded-For to
	// evade the limit or lock out another user.
	//
	// Pick the ClientIPFrom* middleware that matches your deployment; here we
	// assume one trusted proxy in 10.0.0.0/8.
	r.Route("/proxied", func(r chi.Router) {
		r.Use(middleware.ClientIPFromXFF("10.0.0.0/8"))
		r.Use(httprate.LimitBy(100, time.Minute,
			httprate.KeyFromContext(middleware.GetClientIP),
		))

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("proxied: 100 req/min per trusted client IP\n"))
		})
	})

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
		if loginRateLimiter.RespondOnLimit(w, r, payload.Username) {
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
	log.Printf(`curl -v -H 'X-Forwarded-For: 1.2.3.4, 203.0.113.5' http://localhost:3333/proxied?[1-102]`)

	http.ListenAndServe(":3333", r)
}
