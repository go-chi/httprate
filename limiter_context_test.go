package httprate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/httprate"
)

type traceKey struct{}

type recordingCtxCounter struct {
	mu         sync.Mutex
	gets       int
	increments int
	ctxs       []context.Context
}

func (c *recordingCtxCounter) Config(int, time.Duration) {}

func (c *recordingCtxCounter) Increment(key string, currentWindow time.Time) error {
	return c.IncrementBy(key, currentWindow, 1)
}

func (c *recordingCtxCounter) IncrementBy(string, time.Time, int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.increments++
	return nil
}

func (c *recordingCtxCounter) Get(string, time.Time, time.Time) (int, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets++
	return 0, 0, nil
}

func (c *recordingCtxCounter) IncrementContext(ctx context.Context, key string, currentWindow time.Time) error {
	return c.IncrementByContext(ctx, key, currentWindow, 1)
}

func (c *recordingCtxCounter) IncrementByContext(ctx context.Context, key string, currentWindow time.Time, amount int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.increments++
	c.ctxs = append(c.ctxs, ctx)
	return nil
}

func (c *recordingCtxCounter) GetContext(ctx context.Context, key string, currentWindow, previousWindow time.Time) (int, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets++
	c.ctxs = append(c.ctxs, ctx)
	return 0, 0, nil
}

type recordingLegacyCounter struct {
	increments int
}

func (c *recordingLegacyCounter) Config(int, time.Duration) {}
func (c *recordingLegacyCounter) Increment(string, time.Time) error {
	return c.IncrementBy("", time.Time{}, 1)
}
func (c *recordingLegacyCounter) IncrementBy(string, time.Time, int) error {
	c.increments++
	return nil
}
func (c *recordingLegacyCounter) Get(string, time.Time, time.Time) (int, int, error) {
	return 0, 0, nil
}

func TestLimitCounterContextReceivesRequestValuesWithoutCancel(t *testing.T) {
	counter := &recordingCtxCounter{}
	rl := httprate.NewRateLimiter(10, time.Minute, httprate.WithLimitCounter(counter))

	parent, cancel := context.WithCancel(context.WithValue(context.Background(), traceKey{}, "span-1"))
	cancel()

	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent)
	rec := httptest.NewRecorder()
	if rl.OnLimit(rec, req, "k") {
		t.Fatal("OnLimit should not block first request")
	}

	counter.mu.Lock()
	defer counter.mu.Unlock()
	if counter.increments != 1 || counter.gets < 1 {
		t.Fatalf("increments=%d gets=%d, want increment via Context methods", counter.increments, counter.gets)
	}
	if len(counter.ctxs) == 0 {
		t.Fatal("expected context-aware methods to be called")
	}
	for i, ctx := range counter.ctxs {
		if ctx.Err() != nil {
			t.Fatalf("ctxs[%d] canceled: %v (want WithoutCancel)", i, ctx.Err())
		}
		if got, _ := ctx.Value(traceKey{}).(string); got != "span-1" {
			t.Fatalf("ctxs[%d] missing trace value: %q", i, got)
		}
	}
}

func TestLegacyLimitCounterStillWorks(t *testing.T) {
	counter := &recordingLegacyCounter{}
	rl := httprate.NewRateLimiter(10, time.Minute, httprate.WithLimitCounter(counter))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	if rl.OnLimit(rec, req, "k") {
		t.Fatal("OnLimit should not block first request")
	}
	if counter.increments != 1 {
		t.Fatalf("legacy IncrementBy calls = %d, want 1", counter.increments)
	}
}

func TestStatusContextPassesContext(t *testing.T) {
	counter := &recordingCtxCounter{}
	rl := httprate.NewRateLimiter(10, time.Minute, httprate.WithLimitCounter(counter))

	ctx := context.WithValue(context.Background(), traceKey{}, "status")
	if _, _, err := rl.StatusContext(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	counter.mu.Lock()
	defer counter.mu.Unlock()
	if counter.gets == 0 {
		t.Fatal("StatusContext should call GetContext")
	}
	found := false
	for _, c := range counter.ctxs {
		if got, _ := c.Value(traceKey{}).(string); got == "status" {
			found = true
			if c.Err() != nil {
				t.Fatalf("StatusContext ctx canceled: %v", c.Err())
			}
		}
	}
	if !found {
		t.Fatal("StatusContext did not propagate context value")
	}
}
