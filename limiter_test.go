package httprate_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/httprate"
)

func TestLimit(t *testing.T) {
	type test struct {
		name          string
		requestsLimit int
		windowLength  time.Duration
		respCodes     []int
	}
	tests := []test{
		{
			name:          "no-block",
			requestsLimit: 3,
			windowLength:  4 * time.Second,
			respCodes:     []int{200, 200, 200},
		},
		{
			name:          "block",
			requestsLimit: 3,
			windowLength:  2 * time.Second,
			respCodes:     []int{200, 200, 200, 429},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := httprate.LimitAll(tt.requestsLimit, tt.windowLength)(h)

			for i, code := range tt.respCodes {
				req := httptest.NewRequest("GET", "/", nil)
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				if respCode := recorder.Result().StatusCode; respCode != code {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, code)
				}
			}
		})
	}
}

func TestWithIncrement(t *testing.T) {
	type test struct {
		name          string
		increment     int
		requestsLimit int
		respCodes     []int
	}
	tests := []test{
		{
			name:          "no limit",
			increment:     0,
			requestsLimit: 3,
			respCodes:     []int{200, 200, 200, 200},
		},
		{
			name:          "increment 1",
			increment:     1,
			requestsLimit: 3,
			respCodes:     []int{200, 200, 200, 429},
		},
		{
			name:          "increment 2",
			increment:     2,
			requestsLimit: 3,
			respCodes:     []int{200, 429, 429, 429},
		},
		{
			name:          "increment 3",
			increment:     3,
			requestsLimit: 3,
			respCodes:     []int{200, 429, 429, 429},
		},
		{
			name:          "always block",
			increment:     4,
			requestsLimit: 3,
			respCodes:     []int{429, 429, 429, 429},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := httprate.LimitAll(tt.requestsLimit, time.Minute)(h)

			for i, code := range tt.respCodes {
				req := httptest.NewRequest("GET", "/", nil)
				req = req.WithContext(httprate.WithIncrement(req.Context(), tt.increment))
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				if respCode := recorder.Result().StatusCode; respCode != code {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, code)
				}
			}
		})
	}
}

func TestResponseHeaders(t *testing.T) {
	type test struct {
		name                string
		requestsLimit       int
		increments          []int
		respCodes           []int
		respLimitHeader     []string
		respRemainingHeader []string
	}
	tests := []test{
		{
			name:                "const increments",
			requestsLimit:       5,
			increments:          []int{1, 1, 1, 1, 1, 1},
			respCodes:           []int{200, 200, 200, 200, 200, 429},
			respLimitHeader:     []string{"5", "5", "5", "5", "5", "5"},
			respRemainingHeader: []string{"4", "3", "2", "1", "0", "0"},
		},
		{
			name:                "varying increments",
			requestsLimit:       5,
			increments:          []int{2, 2, 1, 2, 10, 1},
			respCodes:           []int{200, 200, 200, 429, 429, 429},
			respLimitHeader:     []string{"5", "5", "5", "5", "5", "5"},
			respRemainingHeader: []string{"3", "1", "0", "0", "0", "0"},
		},
		{
			name:                "no limit",
			requestsLimit:       5,
			increments:          []int{0, 0, 0, 0, 0, 0},
			respCodes:           []int{200, 200, 200, 200, 200, 200},
			respLimitHeader:     []string{"5", "5", "5", "5", "5", "5"},
			respRemainingHeader: []string{"5", "5", "5", "5", "5", "5"},
		},
		{
			name:                "always block",
			requestsLimit:       5,
			increments:          []int{10, 10, 10, 10, 10, 10},
			respCodes:           []int{429, 429, 429, 429, 429, 429},
			respLimitHeader:     []string{"5", "5", "5", "5", "5", "5"},
			respRemainingHeader: []string{"5", "5", "5", "5", "5", "5"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := len(tt.increments)
			if count != len(tt.respCodes) || count != len(tt.respLimitHeader) || count != len(tt.respRemainingHeader) {
				t.Fatalf("invalid test case: increments(%v), respCodes(%v), respLimitHeader(%v) and respRemainingHeaders(%v) must have same size", len(tt.increments), len(tt.respCodes), len(tt.respLimitHeader), len(tt.respRemainingHeader))
			}

			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := httprate.LimitAll(tt.requestsLimit, time.Minute)(h)

			for i := 0; i < count; i++ {
				req := httptest.NewRequest("GET", "/", nil)
				req = req.WithContext(httprate.WithIncrement(req.Context(), tt.increments[i]))
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)

				if respCode := recorder.Result().StatusCode; respCode != tt.respCodes[i] {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, tt.respCodes[i])
				}

				headers := recorder.Result().Header
				if limit := headers.Get("X-RateLimit-Limit"); limit != tt.respLimitHeader[i] {
					t.Errorf("X-RateLimit-Limit(%v) = %v, want %v", i, limit, tt.respLimitHeader[i])
				}
				if remaining := headers.Get("X-RateLimit-Remaining"); remaining != tt.respRemainingHeader[i] {
					t.Errorf("X-RateLimit-Remaining(%v) = %v, want %v", i, remaining, tt.respRemainingHeader[i])
				}

				reset := headers.Get("X-RateLimit-Reset")
				if resetUnixTime, err := strconv.ParseInt(reset, 10, 64); err != nil || resetUnixTime <= time.Now().Unix() {
					t.Errorf("X-RateLimit-Reset(%v) = %v, want unix timestamp in the future", i, reset)
				}
			}
		})
	}
}

func TestCustomResponseHeaders(t *testing.T) {
	type test struct {
		name    string
		headers httprate.ResponseHeaders
	}
	tests := []test{
		{
			name: "no headers",
			headers: httprate.ResponseHeaders{
				Limit:      "",
				Remaining:  "",
				Reset:      "",
				RetryAfter: "",
				Increment:  "",
			},
		},
		{
			name: "custom headers",
			headers: httprate.ResponseHeaders{
				Limit:      "RateLimit-Limit",
				Remaining:  "RateLimit-Remaining",
				Reset:      "RateLimit-Reset",
				RetryAfter: "RateLimit-Retry",
				Increment:  "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := httprate.Limit(
				1,
				time.Minute,
				httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "Wow Slow Down Kiddo", 429)
				}),
				httprate.WithResponseHeaders(tt.headers),
			)(h)

			req := httptest.NewRequest("GET", "/", nil)

			// Force Retry-After and X-RateLimit-Increment headers.
			req = req.WithContext(httprate.WithIncrement(req.Context(), 2))

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			headers := recorder.Result().Header

			for _, header := range []string{
				"X-RateLimit-Limit",
				"X-RateLimit-Remaining",
				"X-RateLimit-Increment",
				"X-RateLimit-Reset",
				"Retry-After",
				"", // ensure we don't set header with an empty key
			} {
				if len(headers.Values(header)) != 0 {
					t.Errorf("%q header not expected", header)
				}
			}

			for _, header := range []string{
				tt.headers.Limit,
				tt.headers.Remaining,
				tt.headers.Increment,
				tt.headers.Reset,
				tt.headers.RetryAfter,
			} {
				if header == "" {
					continue
				}
				if h := headers.Get(header); h == "" {
					t.Errorf("%q header expected", header)
				}
			}
		})
	}
}

func TestLimitHandler(t *testing.T) {
	type test struct {
		name          string
		requestsLimit int
		windowLength  time.Duration
		responses     []struct {
			Body       string
			StatusCode int
		}
	}
	tests := []test{
		{
			name:          "no-block",
			requestsLimit: 3,
			windowLength:  4 * time.Second,
			responses: []struct {
				Body       string
				StatusCode int
			}{
				{Body: "", StatusCode: 200},
				{Body: "", StatusCode: 200},
				{Body: "", StatusCode: 200},
			},
		},
		{
			name:          "block",
			requestsLimit: 3,
			windowLength:  2 * time.Second,
			responses: []struct {
				Body       string
				StatusCode int
			}{
				{Body: "", StatusCode: 200},
				{Body: "", StatusCode: 200},
				{Body: "", StatusCode: 200},
				{Body: "Wow Slow Down Kiddo", StatusCode: 429},
			},
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := httprate.Limit(
				tt.requestsLimit,
				tt.windowLength,
				httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "Wow Slow Down Kiddo", 429)
				}),
			)(h)

			for _, expected := range tt.responses {
				req := httptest.NewRequest("GET", "/", nil)
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				result := recorder.Result()
				if respStatus := result.StatusCode; respStatus != expected.StatusCode {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respStatus, expected.StatusCode)
				}
				buf := new(bytes.Buffer)
				buf.ReadFrom(result.Body)
				respBody := strings.TrimSuffix(buf.String(), "\n")

				if respBody != expected.Body {
					t.Errorf("resp.Body(%v) = %v, want %v", i, respBody, expected.Body)
				}
			}
		})
	}
}

func TestLimitIP(t *testing.T) {
	type test struct {
		name          string
		requestsLimit int
		windowLength  time.Duration
		reqIp         []string
		respCodes     []int
	}
	tests := []test{
		{
			name:          "no-block",
			requestsLimit: 3,
			windowLength:  2 * time.Second,
			reqIp:         []string{"1.1.1.1:100", "2.2.2.2:200"},
			respCodes:     []int{200, 200},
		},
		{
			name:          "block-ip",
			requestsLimit: 1,
			windowLength:  2 * time.Second,
			reqIp:         []string{"1.1.1.1:100", "1.1.1.1:100", "2.2.2.2:200"},
			respCodes:     []int{200, 429, 200},
		},
		{
			name:          "block-ipv6",
			requestsLimit: 1,
			windowLength:  2 * time.Second,
			reqIp:         []string{"2001:DB8::21f:5bff:febf:ce22:1111", "2001:DB8::21f:5bff:febf:ce22:2222", "2002:DB8::21f:5bff:febf:ce22:1111"},
			respCodes:     []int{200, 429, 200},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := httprate.LimitByIP(tt.requestsLimit, tt.windowLength)(h)

			for i, code := range tt.respCodes {
				req := httptest.NewRequest("GET", "/", nil)
				req.RemoteAddr = tt.reqIp[i]
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				if respCode := recorder.Result().StatusCode; respCode != code {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, code)
				}
			}
		})
	}
}

func TestOverrideRequestLimit(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	router := httprate.Limit(
		3,
		time.Minute,
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Wow Slow Down Kiddo", 429)
		}),
	)(h)

	responses := []struct {
		StatusCode   int
		Body         string
		RequestLimit int // Default: 3
	}{
		{StatusCode: 200, Body: ""},
		{StatusCode: 429, Body: "Wow Slow Down Kiddo", RequestLimit: 1},
		{StatusCode: 200, Body: ""},
		{StatusCode: 200, Body: ""},
		{StatusCode: 429, Body: "Wow Slow Down Kiddo"},

		{StatusCode: 200, Body: "", RequestLimit: 5},
		{StatusCode: 200, Body: "", RequestLimit: 5},
		{StatusCode: 429, Body: "Wow Slow Down Kiddo", RequestLimit: 5},
	}
	for i, response := range responses {
		ctx := context.Background()
		if response.RequestLimit > 0 {
			ctx = httprate.WithRequestLimit(ctx, response.RequestLimit)
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "/", nil)
		if err != nil {
			t.Errorf("failed = %v", err)
		}

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		result := recorder.Result()
		if respStatus := result.StatusCode; respStatus != response.StatusCode {
			t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respStatus, response.StatusCode)
		}
		body, _ := io.ReadAll(result.Body)
		respBody := strings.TrimSuffix(string(body), "\n")

		if respBody != response.Body {
			t.Errorf("resp.Body(%v) = %q, want %q", i, respBody, response.Body)
		}
	}
}
