package web

import (
	"net/http"
	"testing"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name       string
		trustProxy bool
		remoteAddr string
		xff        string // "" means the header is not set at all
		want       string
	}{
		{
			name:       "untrusted: XFF ignored even if present",
			trustProxy: false,
			remoteAddr: "203.0.113.9:51234",
			xff:        "1.2.3.4",
			want:       "203.0.113.9",
		},
		{
			name:       "untrusted, no XFF: RemoteAddr",
			trustProxy: false,
			remoteAddr: "203.0.113.9:51234",
			want:       "203.0.113.9",
		},
		{
			name:       "trusted, single-hop XFF: that entry",
			trustProxy: true,
			remoteAddr: "10.0.0.5:443", // our proxy's own connecting address
			xff:        "198.51.100.7",
			want:       "198.51.100.7",
		},
		{
			name:       "trusted, spoofed leading entries: rightmost (our proxy's own hop) wins",
			trustProxy: true,
			remoteAddr: "10.0.0.5:443",
			xff:        "9.9.9.9, 198.51.100.7", // 9.9.9.9 is attacker-supplied
			want:       "198.51.100.7",
		},
		{
			name:       "trusted, extra whitespace in XFF is trimmed",
			trustProxy: true,
			remoteAddr: "10.0.0.5:443",
			xff:        "9.9.9.9 ,  198.51.100.7  ",
			want:       "198.51.100.7",
		},
		{
			name:       "trusted, XFF absent: falls back to RemoteAddr",
			trustProxy: true,
			remoteAddr: "203.0.113.9:51234",
			want:       "203.0.113.9",
		},
		{
			name:       "trusted, unparseable rightmost entry: falls back to RemoteAddr",
			trustProxy: true,
			remoteAddr: "203.0.113.9:51234",
			xff:        "not-an-ip",
			want:       "203.0.113.9",
		},
		{
			name:       "IPv6 RemoteAddr: port stripped",
			trustProxy: false,
			remoteAddr: "[2001:db8::1]:8443",
			want:       "2001:db8::1",
		},
		{
			name:       "RemoteAddr without a port: returned as-is",
			trustProxy: false,
			remoteAddr: "203.0.113.9",
			want:       "203.0.113.9",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: c.remoteAddr, Header: make(http.Header)}
			if c.xff != "" {
				r.Header.Set(ForwardedForHeader, c.xff)
			}
			if got := ClientIP(r, c.trustProxy); got != c.want {
				t.Errorf("ClientIP() = %q, want %q", got, c.want)
			}
		})
	}
}
