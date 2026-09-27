package web

import (
	"net"
	"net/http"
	"strings"
)

// ForwardedForHeader is the de-facto standard header a reverse proxy sets to
// record the client IP it observed, in case a chain of proxies is involved.
const ForwardedForHeader = "X-Forwarded-For"

// ClientIP resolves the caller's IP address, for logging and for keying the
// rate limiter (docs/DECISIONS.md "Rate limiting").
//
// trustProxy gates whether X-Forwarded-For is read at all: false treats it as
// attacker-controlled input and uses RemoteAddr instead, since trusting it
// would let a caller evade an IP-keyed limit by sending a fresh value on
// every request. True trusts only the rightmost entry — the hop our own
// proxy appended, which a client can only ever prepend forged entries in
// front of. Full rule, including the chained-proxy limit: docs/NOTES.md
// "TRUST_PROXY decides what the client IP is".
//
// Falls back to RemoteAddr whenever the header is absent, empty, or its
// rightmost entry doesn't parse as an IP.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get(ForwardedForHeader); xff != "" {
			parts := strings.Split(xff, ",")
			last := strings.TrimSpace(parts[len(parts)-1])
			if ip := net.ParseIP(last); ip != nil {
				return ip.String()
			}
		}
	}
	return remoteIP(r.RemoteAddr)
}

// remoteIP strips the port from a host:port (or [host]:port for IPv6)
// RemoteAddr. Falls back to the raw string if it doesn't parse — net/http
// always sets a host:port RemoteAddr in practice, but a malformed one is more
// useful logged as-is than dropped.
func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
