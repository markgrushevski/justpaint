package web

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Recover turns a panic in any handler into a logged 500 error-envelope response
// instead of a bare stack trace and a dropped connection. Wrap it INSIDE
// LogRequests so the access log sees the 500 it writes, and so the request id
// LogRequests already assigned is in r's context by the time we read it here.
func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				reqID, _ := RequestID(r.Context())
				logger.Error("panic recovered",
					"err", rec, "method", r.Method, "path", r.URL.Path, "request_id", reqID)
				Error(w, http.StatusInternalServerError, CodeInternal, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// LogRequests emits one structured access-log line per request (method, path,
// status, duration, request_id, client_ip). Logs via defer so a panic is
// still recorded.
//
// It also OWNS request-id assignment for the whole handler chain: it resolves
// (or, when trustProxy is true, adopts an inbound X-Request-Id) an id, echoes
// it on the response header, and stores it in the request context BEFORE
// calling next — so every downstream handler (including Recover, meant to
// wrap INSIDE this) can read the same id via RequestID(ctx). trustProxy gates
// both that adoption and the client-ip resolution (ClientIP): both read
// headers that only a trusted reverse proxy in front of us should be allowed
// to set (docs/NOTES.md; docs/IDEAS.md "Request-id correlation").
func LogRequests(logger *slog.Logger, trustProxy bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		reqID := resolveRequestID(r, trustProxy)
		w.Header().Set(RequestIDHeader, reqID)
		r = r.WithContext(context.WithValue(r.Context(), requestIDCtxKey{}, reqID))
		clientIP := ClientIP(r, trustProxy)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", reqID,
				"client_ip", clientIP,
			)
		}()
		next.ServeHTTP(rec, r)
	})
}

// statusRecorder remembers the status code written to the response so the access
// log can report it. WriteHeader defaults to 200 if never called.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the wrapped ResponseWriter so callers that need an optional interface
// the recorder doesn't itself implement can reach the real writer — via the Go 1.20+
// Unwrap convention (http.ResponseController, and coder/websocket's hijacker follow it).
// Critically this restores http.Hijacker for the WS upgrade: without it websocket.Accept
// cannot hijack the connection and the handshake fails 501 (docs/NOTES.md "WS realtime").
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
