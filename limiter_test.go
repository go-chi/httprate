package httprate_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/httprate"
)

var LimiterHeaders = []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "Retry-After"}

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
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := httprate.LimitAll(tt.requestsLimit, tt.windowLength, true)(h)

			for _, code := range tt.respCodes {
				req := httptest.NewRequest("GET", "/", nil)
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)

				response := recorder.Result()

				if respCode := response.StatusCode; respCode != code {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, code)
				}
			}
		})
	}
}

func TestLimitNoHeaders(t *testing.T) {
	type test struct {
		name          string
		requestsLimit int
		windowLength  time.Duration
		respCodes     []int
		headers       []string
	}
	tests := []test{
		{
			name:          "no-block",
			requestsLimit: 3,
			windowLength:  4 * time.Second,
			respCodes:     []int{200, 200, 200},
			headers:       LimiterHeaders,
		},
		{
			name:          "block",
			requestsLimit: 3,
			windowLength:  2 * time.Second,
			respCodes:     []int{200, 200, 200, 429},
			headers:       LimiterHeaders,
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := httprate.LimitAll(tt.requestsLimit, tt.windowLength, false)(h)

			for _, code := range tt.respCodes {
				req := httptest.NewRequest("GET", "/", nil)
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				response := recorder.Result()

				if respCode := response.StatusCode; respCode != code {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, code)
				}

				for _, header := range tt.headers {
					if response.Header.Get(header) != "" {
						t.Errorf("resp.Headers(%v) got %v wanted no header", i, header)
					}
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
				httprate.WithHeaders(true),
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

func TestLimitHandlerNoHeaders(t *testing.T) {
	type test struct {
		name          string
		requestsLimit int
		windowLength  time.Duration
		responses     []struct {
			Body       string
			StatusCode int
			Headers    []string
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
				Headers    []string
			}{
				{Body: "", StatusCode: 200, Headers: LimiterHeaders},
				{Body: "", StatusCode: 200, Headers: LimiterHeaders},
				{Body: "", StatusCode: 200, Headers: LimiterHeaders},
			},
		},
		{
			name:          "block",
			requestsLimit: 3,
			windowLength:  2 * time.Second,
			responses: []struct {
				Body       string
				StatusCode int
				Headers    []string
			}{
				{Body: "", StatusCode: 200, Headers: LimiterHeaders},
				{Body: "", StatusCode: 200, Headers: LimiterHeaders},
				{Body: "", StatusCode: 200, Headers: LimiterHeaders},
				{Body: "Wow Slow Down Kiddo", StatusCode: 429, Headers: LimiterHeaders},
			},
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := httprate.Limit(
				tt.requestsLimit,
				tt.windowLength,
				httprate.WithHeaders(false),
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

				for _, header := range expected.Headers {
					if result.Header.Get(header) != "" {
						t.Errorf("resp.Headers(%v) got %v wanted no header", i, header)
					}
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
			router := httprate.LimitByIP(tt.requestsLimit, tt.windowLength, true)(h)

			for i, code := range tt.respCodes {
				req := httptest.NewRequest("GET", "/", nil)
				req.RemoteAddr = tt.reqIp[i]
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)

				response := recorder.Result()

				if respCode := response.StatusCode; respCode != code {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, code)
				}
			}
		})
	}
}

func TestLimitIPNoHeaders(t *testing.T) {
	type test struct {
		name          string
		requestsLimit int
		windowLength  time.Duration
		reqIp         []string
		respCodes     []int
		headers       []string
	}
	tests := []test{
		{
			name:          "no-block",
			requestsLimit: 3,
			windowLength:  2 * time.Second,
			reqIp:         []string{"1.1.1.1:100", "2.2.2.2:200"},
			respCodes:     []int{200, 200},
			headers:       LimiterHeaders,
		},
		{
			name:          "block-ip",
			requestsLimit: 1,
			windowLength:  2 * time.Second,
			reqIp:         []string{"1.1.1.1:100", "1.1.1.1:100", "2.2.2.2:200"},
			respCodes:     []int{200, 429, 200},
			headers:       LimiterHeaders,
		},
		{
			name:          "block-ipv6",
			requestsLimit: 1,
			windowLength:  2 * time.Second,
			reqIp:         []string{"2001:DB8::21f:5bff:febf:ce22:1111", "2001:DB8::21f:5bff:febf:ce22:2222", "2002:DB8::21f:5bff:febf:ce22:1111"},
			respCodes:     []int{200, 429, 200},
			headers:       LimiterHeaders,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := httprate.LimitByIP(tt.requestsLimit, tt.windowLength, false)(h)

			for i, code := range tt.respCodes {
				req := httptest.NewRequest("GET", "/", nil)
				req.RemoteAddr = tt.reqIp[i]
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)

				response := recorder.Result()

				if respCode := response.StatusCode; respCode != code {
					t.Errorf("resp.StatusCode(%v) = %v, want %v", i, respCode, code)
				}

				for _, header := range tt.headers {
					if response.Header.Get(header) != "" {
						t.Errorf("resp.Headers(%v) got %v wanted no header", i, header)
					}
				}
			}
		})
	}
}
