package httprate

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLimit(t *testing.T) {
	type test struct {
		name      string
		b         int
		respCodes []int
	}
	tests := []test{
		{
			name:      "no-block",
			b:         3,
			respCodes: []int{200, 200, 200},
		},
		{
			name:      "block",
			b:         3,
			respCodes: []int{200, 200, 200, 429},
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := LimitAll(1, tt.b)(h)

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
		name      string
		b         int
		reqIp     []string
		respCodes []int
	}
	tests := []test{
		{
			name:      "no-block",
			b:         1,
			reqIp:     []string{"1.1.1.1:100", "2.2.2.2:200"},
			respCodes: []int{200, 200},
		},
		{
			name:      "block-ip",
			b:         1,
			reqIp:     []string{"1.1.1.1:100", "1.1.1.1:100", "2.2.2.2:200"},
			respCodes: []int{200, 429, 200},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			router := LimitByIP(1, tt.b)(h)

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
