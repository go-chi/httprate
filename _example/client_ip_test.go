package main

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

// clientIPKey mirrors the inline rate-limit key funcs in main.go: the trusted
// client IP resolved by an upstream middleware.ClientIPFrom*, with IPv6 bucketed
// to its /64. Shared here so each test exercises the exact key the example uses.
func clientIPKey(r *http.Request) (string, error) {
	ip := middleware.GetClientIPAddr(r.Context())
	if !ip.IsValid() {
		return "", nil
	}
	if ip.Is4() {
		return ip.String(), nil
	}
	return netip.PrefixFrom(ip, 64).Masked().Addr().String(), nil // IPv6 → /64
}

// These are integration tests for the safe client-IP rate-limiting pattern:
// chi's middleware.ClientIPFrom* resolves a trusted client IP into the request
// context, and httprate.LimitBy(..., KeyFromContext(middleware.GetClientIP))
// rate-limits by it. They live in the _example module on purpose so the main
// httprate go.mod stays chi-free — chi is only a dependency of this example.

// TestLimitByClientIP_RemoteAddr is the happy path: a server directly on the
// internet uses middleware.ClientIPFromRemoteAddr upstream and rate-limits by
// the resolved client IP. The same RemoteAddr is limited after requestLimit
// requests.
func TestLimitByClientIP_RemoteAddr(t *testing.T) {
	h := chain(
		middleware.ClientIPFromRemoteAddr,
		httprate.LimitBy(2, time.Minute, clientIPKey),
		okHandler(),
	)

	// Same client: 3rd request is blocked.
	assertCodes(t, h, requestsFrom("1.2.3.4:1111", 3), []int{200, 200, 429})

	// A different client gets its own bucket.
	assertCodes(t, h, requestsFrom("5.6.7.8:2222", 1), []int{200})
}

// TestLimitByClientIP_IPv6Bucket proves clientIPKey buckets IPv6 clients by
// their /64: addresses that differ only within the /64 share one bucket, so a
// client cannot rotate within its own /64 (trivial with SLAAC) to win fresh
// rate-limit buckets. A different /64 gets its own bucket. Without the /64
// canonicalization, GetClientIP returns the full address and each of these
// would be a separate key — the bypass this guards against.
func TestLimitByClientIP_IPv6Bucket(t *testing.T) {
	h := chain(
		middleware.ClientIPFromRemoteAddr,
		httprate.LimitBy(2, time.Minute, clientIPKey),
		okHandler(),
	)

	// Three addresses in the same /64: the 3rd is blocked.
	codes := make([]int, 0, 3)
	for _, addr := range []string{
		"[2001:db8:abcd:1234::1]:5555",
		"[2001:db8:abcd:1234:ffff::2]:6666",
		"[2001:db8:abcd:1234::3]:7777",
	} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Result().StatusCode)
	}
	wantCodes(t, codes, []int{200, 200, 429})

	// A different /64 has its own bucket.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[2001:db8:abcd:5678::1]:8888"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if code := rec.Result().StatusCode; code != 200 {
		t.Fatalf("different /64 = %d, want 200 (separate bucket)", code)
	}
}

// TestLimitByClientIP_SpoofedXFF is the fix-in-action test. The server sits
// behind a trusted proxy in 10.0.0.0/8 (ClientIPFromXFF). An attacker at
// 203.0.113.5 prepends a forged X-Forwarded-For value on every request, hoping
// to rotate their rate-limit key and evade the limit. But the trusted proxy
// always appends the attacker's real IP (203.0.113.5) as the rightmost XFF
// entry, and ClientIPFromXFF resolves the rightmost non-trusted IP — so the key
// stays 203.0.113.5 no matter what the attacker forges. The 3rd request is
// blocked. This is the test that proves the GHSA is closed.
func TestLimitByClientIP_SpoofedXFF(t *testing.T) {
	h := chain(
		middleware.ClientIPFromXFF("10.0.0.0/8"),
		httprate.LimitBy(2, time.Minute, clientIPKey),
		okHandler(),
	)

	codes := make([]int, 0, 3)
	for _, spoof := range []string{"1.2.3.4", "9.9.9.9", "8.8.8.8"} {
		req := httptest.NewRequest("GET", "/", nil)
		// RemoteAddr is the trusted proxy; the proxy appended the attacker's
		// real IP (203.0.113.5) to whatever XFF value the attacker forged.
		req.RemoteAddr = "10.0.0.1:80"
		req.Header.Set("X-Forwarded-For", spoof+", 203.0.113.5")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Result().StatusCode)
	}
	wantCodes(t, codes, []int{200, 200, 429})

	// Sanity check the contrast: with the spoofable KeyByRealIP, rotating the
	// forged header would have yielded a fresh key each time (no 429), which is
	// exactly the evasion this fix prevents.
	spoofedKey, _ := httprate.KeyByRealIP(mustReq("10.0.0.1:80", "1.2.3.4, 203.0.113.5"))
	if spoofedKey != "1.2.3.4" {
		t.Fatalf("sanity: KeyByRealIP = %q, want spoofed %q", spoofedKey, "1.2.3.4")
	}
}

// TestLimitByClientIP_Misconfig documents the silent-global-bucket footgun: if
// no ClientIPFrom* middleware is installed, GetClientIP returns "" for every
// request, so all clients share a single bucket. This is strictly more
// restrictive (never less), so it is a developer footgun, not a security
// regression.
func TestLimitByClientIP_Misconfig(t *testing.T) {
	h := chain(
		// No middleware.ClientIPFrom* installed on purpose.
		httprate.LimitBy(2, time.Minute, clientIPKey),
		okHandler(),
	)

	// Three requests from three different sources with different spoofed XFFs
	// still share one bucket: the 3rd is blocked regardless of source.
	codes := make([]int, 0, 3)
	srcs := []string{"1.1.1.1:1", "2.2.2.2:2", "3.3.3.3:3"}
	for _, src := range srcs {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = src
		req.Header.Set("X-Forwarded-For", src)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Result().StatusCode)
	}
	wantCodes(t, codes, []int{200, 200, 429})
}

// chain composes the given middlewares around a final handler, outermost first.
func chain(parts ...interface{}) http.Handler {
	if len(parts) == 0 {
		return http.NotFoundHandler()
	}
	h, ok := parts[len(parts)-1].(http.Handler)
	if !ok {
		panic("chain: last element must be an http.Handler")
	}
	for i := len(parts) - 2; i >= 0; i-- {
		mw, ok := parts[i].(func(http.Handler) http.Handler)
		if !ok {
			panic("chain: middleware must be func(http.Handler) http.Handler")
		}
		h = mw(h)
	}
	return h
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func mustReq(remoteAddr, xff string) *http.Request {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = remoteAddr
	req.Header.Set("X-Forwarded-For", xff)
	return req
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
