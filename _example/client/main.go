package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/httprate"
	"github.com/go-chi/transport"
)

type loginPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func main() {
	ctx := context.Background()
	cl := &http.Client{
		Transport: transport.Chain(http.DefaultTransport, httprate.RateLimitedRequest(httprate.RPMLimit(10), 1)),
	}

	req := &loginPayload{
		Username: "alice",
		Password: "password",
	}

	payload, err := json.Marshal(req)
	if err != nil {
		log.Fatal(err)
	}

	for {
		func() {
			ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			// login accepts only 5req/mint so it gets rate limited eventually
			req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:3333/login", bytes.NewReader(payload))
			if err != nil {
				log.Fatal(err)
			}

			fmt.Println("request started", time.Now())

			resp, err := cl.Do(req)
			if err != nil {
				log.Fatal(err)
			}

			defer resp.Body.Close()

			if resp.StatusCode == http.StatusTooManyRequests {
				fmt.Println("rate limited")
			}

			fmt.Println()
		}()
	}
}
