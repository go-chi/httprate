package httprate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/httprate"
)

// TestKeyFunc_FromContext verifies that LimitBy keys off whatever a KeyFunc
// reads from the request context: requests carrying the same value share a
// bucket, different values get their own. A KeyFunc receives *http.Request, so
// it reads r.Context() directly — no special context helper needed.
//
// Integration with chi's middleware.ClientIPFrom* / middleware.GetClientIP
// lives in _example/client_ip_test.go so the main module stays chi-free.
func TestKeyFunc_FromContext(t *testing.T) {
	type ctxKey string
	const tenantKey ctxKey = "tenant"

	keyByTenant := func(r *http.Request) (string, error) {
		v, _ := r.Context().Value(tenantKey).(string)
		return v, nil
	}

	withTenant := func(tenant string, next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tenantKey, tenant)))
		})
	}

	limiter := httprate.LimitBy(2, time.Minute, keyByTenant)

	get := func(tenant string) int {
		h := withTenant(tenant, limiter(okHandler()))
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Result().StatusCode
	}

	// Same tenant trips the limit on the 3rd request.
	wantCodes(t, []int{get("acme"), get("acme"), get("acme")}, []int{200, 200, 429})
	// A different tenant has its own bucket.
	if code := get("globex"); code != 200 {
		t.Fatalf("different tenant = %d, want 200 (separate bucket)", code)
	}
}

// TestKeyFunc_EmptyKey documents the silent-global-bucket footgun: when the
// KeyFunc returns "" (e.g. the upstream middleware that should populate the
// context was not installed), every request shares one bucket. Strictly more
// restrictive, not a security regression.
func TestKeyFunc_EmptyKey(t *testing.T) {
	emptyKey := func(r *http.Request) (string, error) { return "", nil }

	h := httprate.LimitBy(2, time.Minute, emptyKey)(okHandler())

	// Three different sources still share one bucket; the 3rd is blocked.
	codes := make([]int, 0, 3)
	for _, src := range []string{"1.1.1.1:1", "2.2.2.2:2", "3.3.3.3:3"} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = src
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Result().StatusCode)
	}
	wantCodes(t, codes, []int{200, 200, 429})
}

// TestJoinKeys verifies multi-dimensional rate-limiting: the same IP hitting
// two different endpoints gets two independent buckets.
func TestJoinKeys(t *testing.T) {
	h := httprate.LimitBy(1, time.Minute, httprate.JoinKeys(httprate.KeyByIP, httprate.KeyByEndpoint))(okHandler())

	get := func(path string) int {
		req := httptest.NewRequest("GET", path, nil)
		req.RemoteAddr = "1.2.3.4:1111"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Result().StatusCode
	}

	if code := get("/a"); code != 200 {
		t.Fatalf("GET /a #1 = %d, want 200", code)
	}
	if code := get("/b"); code != 200 {
		t.Fatalf("GET /b #1 = %d, want 200 (different endpoint, separate bucket)", code)
	}
	if code := get("/a"); code != 429 {
		t.Fatalf("GET /a #2 = %d, want 429 (same IP+endpoint bucket exhausted)", code)
	}
}

// TestJoinKeys_EmptyComponent verifies that an empty key component (e.g. an
// unauthenticated request with no tenant) is passed through without error or
// panic — the empty component just becomes part of the joined key.
func TestJoinKeys_EmptyComponent(t *testing.T) {
	keyByTenant := func(r *http.Request) (string, error) {
		v, _ := r.Context().Value("tenant").(string)
		return v, nil
	}

	h := httprate.LimitBy(2, time.Minute, httprate.JoinKeys(httprate.KeyByIP, keyByTenant))(okHandler())

	// No "tenant" in context: empty component, no error, request still served.
	assertCodes(t, h, requestsFrom("1.2.3.4:1111", 3), []int{200, 200, 429})
}

// TestKeyByRealIP_SpoofableRegression documents the deprecated, spoofable
// behavior of KeyByRealIP: a client-supplied True-Client-IP header dictates the
// rate-limit key with no proxy-trust check. This is exactly why KeyByRealIP,
// WithKeyByRealIP, and LimitByRealIP are deprecated (GHSA-9g5q-2w5x-hmxf,
// GHSA-rjr7-jggh-pgcp, GHSA-3fxj-6jh8-hvhx). Kept so future archaeologists
// understand the deprecation.
func TestKeyByRealIP_SpoofableRegression(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.5:4444"
	req.Header.Set("True-Client-IP", "1.2.3.4")

	key, err := httprate.KeyByRealIP(req)
	if err != nil {
		t.Fatalf("KeyByRealIP returned error: %v", err)
	}
	if key != "1.2.3.4" {
		t.Fatalf("KeyByRealIP = %q, want %q (spoofed header trusted — the deprecated behavior)", key, "1.2.3.4")
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func requestsFrom(remoteAddr string, n int) []*http.Request {
	reqs := make([]*http.Request, n)
	for i := range reqs {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = remoteAddr
		reqs[i] = req
	}
	return reqs
}

func assertCodes(t *testing.T, h http.Handler, reqs []*http.Request, want []int) {
	t.Helper()
	got := make([]int, len(reqs))
	for i, req := range reqs {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		got[i] = rec.Result().StatusCode
	}
	wantCodes(t, got, want)
}

func wantCodes(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("status codes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("status code[%d] = %d, want %d (full: got %v, want %v)", i, got[i], want[i], got, want)
		}
	}
}
