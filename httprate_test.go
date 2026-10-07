package httprate

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCanonicalizeIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want string
	}{
		{
			name: "IPv4 unchanged",
			ip:   "1.2.3.4",
			want: "1.2.3.4",
		},
		{
			name: "mapped IPv4 dotted suffix",
			ip:   "::ffff:192.0.2.1",
			want: "192.0.2.1",
		},
		{
			name: "mapped IPv4 hexadecimal suffix",
			ip:   "::ffff:c000:201",
			want: "192.0.2.1",
		},
		{
			name: "mapped IPv4 expanded uppercase",
			ip:   "0000:0000:0000:0000:0000:FFFF:C000:0201",
			want: "192.0.2.1",
		},
		{
			name: "different mapped IPv4",
			ip:   "::ffff:192.0.2.2",
			want: "192.0.2.2",
		},
		{
			name: "mapped unspecified IPv4",
			ip:   "::ffff:0.0.0.0",
			want: "0.0.0.0",
		},
		{
			name: "mapped broadcast IPv4",
			ip:   "::ffff:255.255.255.255",
			want: "255.255.255.255",
		},
		{
			name: "invalid mapped IPv4 unchanged",
			ip:   "::ffff:192.0.2.999",
			want: "::ffff:192.0.2.999",
		},
		{
			name: "IPv4 compatible IPv6",
			ip:   "::192.0.2.1",
			want: "::",
		},
		{
			name: "NAT64 IPv6",
			ip:   "64:ff9b::192.0.2.1",
			want: "64:ff9b::",
		},
		{
			name: "bad IP unchanged",
			ip:   "not an IP",
			want: "not an IP",
		},
		{
			name: "bad IPv6 unchanged",
			ip:   "not:an:IP",
			want: "not:an:IP",
		},
		{
			name: "empty string unchanged",
			ip:   "",
			want: "",
		},
		{
			name: "IPv6 test 1",
			ip:   "2001:DB8::21f:5bff:febf:ce22:8a2e",
			want: "2001:db8:0:21f::",
		},
		{
			name: "IPv6 test 2",
			ip:   "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			want: "2001:db8:85a3::",
		},
		{
			name: "IPv6 test 3",
			ip:   "fe80::1ff:fe23:4567:890a",
			want: "fe80::",
		},
		{
			name: "IPv6 test 4",
			ip:   "f:f:f:f:f:f:f:f",
			want: "f:f:f:f::",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonicalizeIP(tt.ip); got != tt.want {
				t.Errorf("CanonicalizeIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMappedIPv4ClientBuckets(t *testing.T) {
	for _, keyFunc := range []struct {
		name string
		fn   KeyFunc
	}{
		{
			name: "CanonicalizeIP",
			fn: func(r *http.Request) (string, error) {
				return CanonicalizeIP(r.Header.Get("Client-IP")), nil
			},
		},
		{name: "KeyByIP", fn: KeyByIP},
	} {
		t.Run(keyFunc.name, func(t *testing.T) {
			handler := LimitBy(1, time.Hour, keyFunc.fn)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			for _, request := range []struct {
				ip   string
				want int
			}{
				{ip: "192.0.2.1", want: http.StatusOK},
				{ip: "::ffff:c000:201", want: http.StatusTooManyRequests},
				{ip: "::ffff:192.0.2.1", want: http.StatusTooManyRequests},
				{ip: "::ffff:192.0.2.2", want: http.StatusOK},
				{ip: "0000:0000:0000:0000:0000:FFFF:C000:0202", want: http.StatusTooManyRequests},
				{ip: "192.0.2.2", want: http.StatusTooManyRequests},
			} {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Client-IP", request.ip)
				req.RemoteAddr = fmt.Sprintf("[%s]:1234", request.ip)
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, req)
				if got := recorder.Code; got != request.want {
					t.Errorf("client %s: status = %d, want %d", request.ip, got, request.want)
				}
			}
		})
	}
}
