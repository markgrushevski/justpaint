package web

import (
	"context"
	"net/http"
	"regexp"

	"github.com/google/uuid"
)

// RequestIDHeader is echoed on every response — and, when the deployment
// trusts its reverse proxy, adopted from an inbound request — so one request
// can be correlated across the proxy's logs, ours, and the client's
// (docs/IDEAS.md "Request-id correlation").
const RequestIDHeader = "X-Request-Id"

// maxInboundRequestID bounds an ADOPTED inbound id (see resolveRequestID) —
// generous for a uuid or a typical proxy/CDN trace id, small enough to keep a
// hostile header value out of the logs and the echoed response header.
const maxInboundRequestID = 128

// validRequestID matches a conservative token charset for an inbound
// X-Request-Id we are about to adopt and echo back — printable ASCII with no
// whitespace or control characters, so a hostile value can't smuggle anything
// odd into a structured log line or a response header.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// requestIDCtxKey is an unexported type so this context key can't collide
// with a key from another package (context.WithValue best practice — mirrors
// internal/auth's ctxKey).
type requestIDCtxKey struct{}

// RequestID returns the id LogRequests assigned to r's context, if any. Other
// packages that want to attribute their own log line to the same request read
// it from here rather than re-deriving or re-generating one (docs/IDEAS.md
// "Request-id correlation"). internal/platform/web.Recover is the first such
// consumer.
func RequestID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(requestIDCtxKey{}).(string)
	return v, ok
}

// resolveRequestID picks the id for one request: an inbound X-Request-Id is
// adopted ONLY when trustProxy is true AND the value looks like a sane token
// — an untrusted client could otherwise inject an arbitrary id (e.g. to make
// its own abusive requests appear correlated with an unrelated past request
// in our logs, or to smuggle an oversized/odd value into them). Every other
// case gets a freshly generated uuid.
func resolveRequestID(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if id := r.Header.Get(RequestIDHeader); id != "" &&
			len(id) <= maxInboundRequestID && validRequestID.MatchString(id) {
			return id
		}
	}
	return uuid.NewString()
}
