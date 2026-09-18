package web

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestLogger returns a JSON slog.Logger writing into a buffer so tests can
// assert on the structured fields of a log line, the same envelope
// docs/NOTES.md documents this middleware as producing.
func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil)), &buf
}

// decodeLogLines parses every line in buf as one JSON object.
func decodeLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("decode log line: %v (line: %s)", err, line)
		}
		out = append(out, m)
	}
	return out
}

func TestLogRequests_LogsRequestIDAndClientIP(t *testing.T) {
	logger, buf := newTestLogger()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := LogRequests(logger, false, next)

	req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	req.RemoteAddr = "203.0.113.9:4242"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}

	headerID := rec.Header().Get(RequestIDHeader)
	if headerID == "" {
		t.Fatal("response is missing the X-Request-Id header")
	}

	lines := decodeLogLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1", len(lines))
	}
	entry := lines[0]
	if entry["request_id"] != headerID {
		t.Errorf("log request_id = %v, want %q (the echoed header)", entry["request_id"], headerID)
	}
	if entry["client_ip"] != "203.0.113.9" {
		t.Errorf(`log client_ip = %v, want "203.0.113.9"`, entry["client_ip"])
	}
	if entry["status"] != float64(http.StatusTeapot) {
		t.Errorf("log status = %v, want %d", entry["status"], http.StatusTeapot)
	}
}

// TestLogRequests_DownstreamSeesTheSameRequestID pins that the id is available
// to handlers further down the chain via RequestID(ctx) — not just echoed on
// the response — before LogRequests' own deferred log line runs.
func TestLogRequests_DownstreamSeesTheSameRequestID(t *testing.T) {
	logger, _ := newTestLogger()
	var sawID string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		sawID, _ = RequestID(r.Context())
	})
	handler := LogRequests(logger, false, next)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	headerID := rec.Header().Get(RequestIDHeader)
	if sawID == "" || sawID != headerID {
		t.Errorf("downstream RequestID() = %q, want the echoed header %q", sawID, headerID)
	}
}

// TestLogRequests_UntrustedProxyIgnoresInboundRequestID pins the trust gate:
// an inbound X-Request-Id is only ever adopted when trustProxy is true.
func TestLogRequests_UntrustedProxyIgnoresInboundRequestID(t *testing.T) {
	logger, _ := newTestLogger()
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	handler := LogRequests(logger, false, next)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(RequestIDHeader, "attacker-supplied-id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(RequestIDHeader); got == "attacker-supplied-id" {
		t.Error("an untrusted inbound X-Request-Id must not be adopted")
	}
}

// TestLogRequests_TrustedProxyAdoptsInboundRequestID is the mirror image: a
// trusted proxy's id rides through unchanged.
func TestLogRequests_TrustedProxyAdoptsInboundRequestID(t *testing.T) {
	logger, _ := newTestLogger()
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	handler := LogRequests(logger, true, next)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(RequestIDHeader, "upstream-trace-id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(RequestIDHeader); got != "upstream-trace-id" {
		t.Errorf("echoed request id = %q, want the adopted upstream id", got)
	}
}

// TestRecover_LogsRequestID pins that Recover — wrapped INSIDE LogRequests,
// the documented composition — attributes its panic log line to the same
// request id LogRequests assigned and echoed on the response.
func TestRecover_LogsRequestID(t *testing.T) {
	logger, buf := newTestLogger()
	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})
	handler := LogRequests(logger, false, Recover(logger, panicking))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	headerID := rec.Header().Get(RequestIDHeader)
	if headerID == "" {
		t.Fatal("response is missing the X-Request-Id header")
	}

	var panicEntry map[string]any
	for _, entry := range decodeLogLines(t, buf) {
		if entry["msg"] == "panic recovered" {
			panicEntry = entry
		}
	}
	if panicEntry == nil {
		t.Fatal(`expected a "panic recovered" log line`)
	}
	if panicEntry["request_id"] != headerID {
		t.Errorf("panic log request_id = %v, want %q (the id echoed on the response)",
			panicEntry["request_id"], headerID)
	}
}
