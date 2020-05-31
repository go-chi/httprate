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
	r.Use(httprate.LimitByIP(2, 2))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("."))
	})

	http.ListenAndServe(":3333", r)
}
