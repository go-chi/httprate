package httprate

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/transport"
	"golang.org/x/time/rate"
)

func RateLimitedRequest(limit rate.Limit, burst int) func(http.RoundTripper) http.RoundTripper {
	limiter := rate.NewLimiter(limit, burst)

	return func(next http.RoundTripper) http.RoundTripper {
		return transport.RoundTripFunc(func(req *http.Request) (resp *http.Response, err error) {
			if err := limiter.Wait(req.Context()); err != nil {
				return nil, fmt.Errorf("rate limiter wait: %w", err)
			}

			return next.RoundTrip(req)
		})
	}
}

func RPMLimit(rpm uint) rate.Limit {
	return rate.Every(time.Minute / time.Duration(rpm))
}
