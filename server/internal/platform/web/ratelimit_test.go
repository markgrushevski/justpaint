package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/platform/ratelimit"
)

func TestMethodPrefix(t *testing.T) {
	cases := []struct {
		name    string
		prefix  string
		methods []string
		method  string
		path    string
		want    bool
	}{
		{"matches method + prefix", "/api/auth/", []string{http.MethodPost}, http.MethodPost, "/api/auth/login", true},
		{"wrong method", "/api/auth/", []string{http.MethodPost}, http.MethodGet, "/api/auth/login", false},
		{"wrong prefix", "/api/auth/", []string{http.MethodPost}, http.MethodPost, "/api/drawings", false},
		{"no methods given: any method matches", "/api/drawings", nil, http.MethodDelete, "/api/drawings/1", true},
		{"multiple methods: one matches", "/api/matches", []string{http.MethodPost, http.MethodPut, http.MethodDelete}, http.MethodPut, "/api/matches/1", true},
		{"multiple methods: none matches", "/api/matches", []string{http.MethodPost, http.MethodPut, http.MethodDelete}, http.MethodGet, "/api/matches/1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			match := MethodPrefix(c.prefix, c.methods...)
			r := httptest.NewRequest(c.method, c.path, nil)
			if got := match(r); got != c.want {
				t.Errorf("MethodPrefix(%q, %v)(%s %s) = %v, want %v",
					c.prefix, c.methods, c.method, c.path, got, c.want)
			}
		})
	}
}

// errorCode decodes the standard { "error": { "code", "message" } } envelope
// (docs/API.md §3) and returns just the code.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error envelope: %v (body: %s)", err, rec.Body.String())
	}
	return body.Error.Code
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestRateLimit_ThrottlesOnMatchedPolicy(t *testing.T) {
	logger, _ := newTestLogger()
	strict := ratelimit.New(1, time.Minute, 0, 0)
	policies := []RatePolicy{
		{Name: "auth-strict", Match: MethodPrefix("/api/auth/", http.MethodPost), Limiter: strict},
	}
	handler := RateLimit(false, policies, logger)(okHandler())

	newReq := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		r.RemoteAddr = "203.0.113.9:1111"
		return r
	}

	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, newReq())
	if rec1.Code != http.StatusOK {
		t.Fatalf("request 1 status = %d, want 200", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, newReq())
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("request 2 status = %d, want 429", rec2.Code)
	}
	if got := errorCode(t, rec2); got != CodeRateLimited {
		t.Errorf("error code = %q, want %q", got, CodeRateLimited)
	}
	if ra := rec2.Header().Get("Retry-After"); ra == "" {
		t.Error("missing Retry-After header on 429")
	}
}

func TestRateLimit_NonMatchingRequestIsNeverThrottled(t *testing.T) {
	logger, _ := newTestLogger()
	zeroBurst := ratelimit.New(0, time.Minute, 0, 0) // would deny EVERY request it governs
	policies := []RatePolicy{
		{Name: "auth-strict", Match: MethodPrefix("/api/auth/", http.MethodPost), Limiter: zeroBurst},
	}
	handler := RateLimit(false, policies, logger)(okHandler())

	// A GET to an unrelated path matches no policy row.
	req := httptest.NewRequest(http.MethodGet, "/api/drawings", nil)
	req.RemoteAddr = "203.0.113.9:1111"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (no policy row matched, so no throttling applies)", rec.Code)
	}
}

func TestRateLimit_PerClientIPIsolation(t *testing.T) {
	logger, _ := newTestLogger()
	limiter := ratelimit.New(1, time.Minute, 0, 0)
	policies := []RatePolicy{
		{Name: "default", Match: func(*http.Request) bool { return true }, Limiter: limiter},
	}
	handler := RateLimit(false, policies, logger)(okHandler())

	reqFrom := func(ip string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/drawings", nil)
		r.RemoteAddr = ip + ":1111"
		return r
	}

	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, reqFrom("203.0.113.9"))
	if rec1.Code != http.StatusOK {
		t.Fatalf("client A request 1 status = %d, want 200", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, reqFrom("203.0.113.9"))
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("client A request 2 status = %d, want 429 (burst of 1 exhausted)", rec2.Code)
	}

	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, reqFrom("198.51.100.7"))
	if rec3.Code != http.StatusOK {
		t.Fatalf("client B request 1 status = %d, want 200 (an independent bucket)", rec3.Code)
	}
}

func TestRateLimit_FirstMatchWins(t *testing.T) {
	logger, _ := newTestLogger()
	strict := ratelimit.New(0, time.Minute, 0, 0) // denies everything it governs
	generous := ratelimit.New(100, time.Minute, 0, 0)
	policies := []RatePolicy{
		{Name: "strict", Match: MethodPrefix("/api/auth/", http.MethodPost), Limiter: strict},
		{Name: "generous", Match: func(*http.Request) bool { return true }, Limiter: generous},
	}
	handler := RateLimit(false, policies, logger)(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "203.0.113.9:1111"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 (the strict row matches first and must govern, not fall through to generous)", rec.Code)
	}
}
