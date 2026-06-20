package web

import (
	"log/slog"
	"net/http"
	"time"
)

// Recover turns a panic in any handler into a logged 500 error-envelope response
// instead of a bare stack trace and a dropped connection. Wrap it INSIDE
// LogRequests so the access log sees the 500 it writes.
func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered", "err", rec, "method", r.Method, "path", r.URL.Path)
				Error(w, http.StatusInternalServerError, CodeInternal, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// LogRequests emits one structured access-log line per request
// (method, path, status, duration). Logs via defer so a panic is still recorded.
func LogRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
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
