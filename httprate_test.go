package httprate

import "testing"

func Test_canonicalizeIP(t *testing.T) {
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
			if got := canonicalizeIP(tt.ip); got != tt.want {
				t.Errorf("canonicalizeIP() = %v, want %v", got, tt.want)
			}
		})
	}
}
