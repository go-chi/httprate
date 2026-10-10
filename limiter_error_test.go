package httprate_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/httprate"
)

func TestRespondOnLimit_CounterErrorUsesErrorHandlerOnly(t *testing.T) {
	tests := []struct {
		name    string
		counter httprate.LimitCounter
		wantErr string
	}{
		{
			name:    "get",
			counter: failingLimitCounter{failGet: true},
			wantErr: "get failed",
		},
		{
			name:    "increment",
			counter: failingLimitCounter{failIncrement: true},
			wantErr: "increment failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errorHandlerCalls, limitHandlerCalls int
			h := httprate.LimitBy(
				1,
				time.Minute,
				httprate.Key("key"),
				httprate.WithLimitCounter(tt.counter),
				httprate.WithErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
					errorHandlerCalls++
					http.Error(w, "backend error: "+err.Error(), http.StatusServiceUnavailable)
				}),
				httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
					limitHandlerCalls++
					http.Error(w, "rate limited", http.StatusTooManyRequests)
				}),
			)(http.NotFoundHandler())

			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

			if got := recorder.Result().StatusCode; got != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", got, http.StatusServiceUnavailable)
			}
			body, err := io.ReadAll(recorder.Result().Body)
			if err != nil {
				t.Fatalf("read response body: %v", err)
			}
			if got, want := string(body), "backend error: "+tt.wantErr+"\n"; got != want {
				t.Errorf("body = %q, want %q", got, want)
			}
			if errorHandlerCalls != 1 {
				t.Errorf("error handler calls = %d, want 1", errorHandlerCalls)
			}
			if limitHandlerCalls != 0 {
				t.Errorf("limit handler calls = %d, want 0", limitHandlerCalls)
			}
		})
	}
}

type failingLimitCounter struct {
	failGet       bool
	failIncrement bool
}

func (f failingLimitCounter) Config(int, time.Duration) {}

func (f failingLimitCounter) Increment(string, time.Time) error {
	return f.IncrementBy("", time.Time{}, 1)
}

func (f failingLimitCounter) IncrementBy(string, time.Time, int) error {
	if f.failIncrement {
		return errors.New("increment failed")
	}
	return nil
}

func (f failingLimitCounter) Get(string, time.Time, time.Time) (int, int, error) {
	if f.failGet {
		return 0, 0, errors.New("get failed")
	}
	return 0, 0, nil
}
