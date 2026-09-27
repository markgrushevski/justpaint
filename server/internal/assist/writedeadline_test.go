package assist

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// slowAssist answers like the fake, after a delay, unless its context ends first.
type slowAssist struct{ delay time.Duration }

func (s slowAssist) GenerateOps(ctx context.Context, req Request) (Result, error) {
	select {
	case <-time.After(s.delay):
		return NewFakeAssist().GenerateOps(ctx, req)
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

// postThroughServer sends one assist request through a real http.Server whose
// WriteTimeout is shorter than the generation, as in production.
func postThroughServer(t *testing.T, h *Handler, writeTimeout time.Duration) (*http.Response, error) {
	t.Helper()
	ts := httptest.NewUnstartedServer(authMiddleware(t)(http.HandlerFunc(h.GenerateOps)))
	ts.Config.WriteTimeout = writeTimeout
	ts.Start()
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/assist/ops", strings.NewReader(validBody))
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(mintCookie(t, "u1"))
	return ts.Client().Do(req)
}

func newSlowHandler(delay, runBudget time.Duration) *Handler {
	return NewHandler(slowAssist{delay: delay}, NewRateLimiter(DefaultBurst, time.Minute),
		nil, nil, runBudget, slog.New(slog.DiscardHandler))
}

func TestGenerateOps_OutlivesTheServerWriteTimeout(t *testing.T) {
	res, err := postThroughServer(t, newSlowHandler(300*time.Millisecond, 2*time.Second), 100*time.Millisecond)
	if err != nil {
		t.Fatalf("the response was dropped: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}
}

func TestGenerateOps_OverBudgetAnswersInsteadOfDropping(t *testing.T) {
	res, err := postThroughServer(t, newSlowHandler(5*time.Second, 200*time.Millisecond), 100*time.Millisecond)
	if err != nil {
		t.Fatalf("the response was dropped: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", res.StatusCode)
	}
}
