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
// When trustProxy is false (no reverse proxy in front of us, or one we have
// not explicitly vetted), X-Forwarded-For is NEVER read: on a request that
// reaches us directly, the header is just more attacker-controlled input, and
// trusting it would be worse than the coarse-but-honest RemoteAddr — it would
// let one caller evade an IP-keyed rate limit entirely by sending a fresh
// X-Forwarded-For on every request.
//
// When trustProxy is true (the deployment sits behind exactly ONE reverse
// proxy we control — e.g. Render's or Fly's edge load balancer terminating
// TLS directly in front of the app; that single-hop topology is what this
// resolves for), we read X-Forwarded-For but take the RIGHTMOST entry, not
// the leftmost. X-Forwarded-For is built by each hop APPENDING the address of
// whoever it received the connection from, so the leftmost entry is "the
// original client, as claimed by the first hop" — correct with a well-behaved
// chain, but that same leftmost slot is exactly what a client can forge by
// sending its OWN X-Forwarded-For before ever reaching our proxy (e.g.
// "X-Forwarded-For: 1.2.3.4"). Our trusted proxy then APPENDS its own
// observed peer rather than overwriting, so the header becomes
// "1.2.3.4, <real-client-ip>" — attacker-chosen entries only ever get
// prepended to the LEFT. The rightmost entry is always the one hop WE trust:
// the address our own proxy read off the raw TCP connection, which the client
// cannot forge. (A deployment that adds a SECOND chained proxy in front of the
// app — e.g. a CDN in front of the LB — would need to trust the
// second-from-right entry instead; out of scope today, see docs/NOTES.md.)
//
// Falls back to RemoteAddr whenever the header is absent, empty, or its
// rightmost entry doesn't parse as an IP — treating an unexpected shape as
// "no proxy" is safer than trusting garbage.
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
