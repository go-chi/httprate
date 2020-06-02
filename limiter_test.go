package httprate_test

import (
	"net/http"
	"net/http/httptest"
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
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := httprate.LimitAll(tt.requestsLimit, tt.windowLength)(h)

			for _, code := range tt.respCodes {
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
