module github.com/go-chi/httprate/_example

go 1.22.5

replace github.com/go-chi/httprate => ../

require (
	github.com/go-chi/chi/v5 v5.1.0
	github.com/go-chi/httprate v0.0.0-00010101000000-000000000000
)

require github.com/cespare/xxhash/v2 v2.3.0 // indirect
