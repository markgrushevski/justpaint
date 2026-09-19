package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeJSON encodes v as the response body. The fixtures below are fixed,
// well-formed Go values, so Encode cannot fail in practice.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestNewHTTPJudge_TrimsTrailingSlash(t *testing.T) {
	hj := NewHTTPJudge("http://example.com/", 5*time.Second)
	if hj.baseURL != "http://example.com" {
		t.Errorf("baseURL = %q, want the trailing slash trimmed", hj.baseURL)
	}
}

// TestHTTPJudge_Score_HappyPath pins the wire shape, not just the outcome: the
// exact request §6 describes, and the exact Result decoded from a well-formed
// 200.
func TestHTTPJudge_Score_HappyPath(t *testing.T) {
	const wantIdemKeyLen = 64 // hex-encoded sha256

	var gotMethod, gotPath, gotContentType, gotVersion, gotIdemKey string
	var gotBody httpWireRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotVersion = r.Header.Get("X-Judge-Contract-Version")
		gotIdemKey = r.Header.Get("Idempotency-Key")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		writeJSON(w, http.StatusOK, httpWireResponse{
			ScoreA: 0.82, ScoreB: 0.61, Winner: WinnerA,
			Reason: "A clearly shows the prompt; B reads as an abstract blob.",
		})
	}))
	defer server.Close()

	// The trailing slash on the base URL must not produce a doubled slash
	// before /v1/score.
	hj := NewHTTPJudge(server.URL+"/", time.Second)
	req := Request{Prompt: "a fox riding a bicycle", ImageA: []byte{1, 2, 3}, ImageB: []byte{4, 5, 6, 7}}
	got, err := hj.Score(context.Background(), req)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	want := Result{ScoreA: 0.82, ScoreB: 0.61, Winner: WinnerA, Reason: "A clearly shows the prompt; B reads as an abstract blob."}
	if got != want {
		t.Errorf("Score = %+v, want %+v", got, want)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/score" {
		t.Errorf("path = %q, want /v1/score", gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotVersion != "1" {
		t.Errorf("X-Judge-Contract-Version = %q, want 1", gotVersion)
	}
	if len(gotIdemKey) != wantIdemKeyLen {
		t.Errorf("Idempotency-Key = %q, want a %d-char hex digest", gotIdemKey, wantIdemKeyLen)
	}
	if gotBody.Prompt != req.Prompt {
		t.Errorf("request prompt = %q, want %q", gotBody.Prompt, req.Prompt)
	}
	if !bytes.Equal(gotBody.ImageA, req.ImageA) {
		t.Errorf("request imageA = %v, want %v", gotBody.ImageA, req.ImageA)
	}
	if !bytes.Equal(gotBody.ImageB, req.ImageB) {
		t.Errorf("request imageB = %v, want %v", gotBody.ImageB, req.ImageB)
	}
}

// TestHTTPJudge_Score_RetryBehavior covers the three attempt-count rules §7
// pins in one table: a transient failure recovers, a persistent one exhausts
// the budget, and a 4xx never gets a second try.
func TestHTTPJudge_Score_RetryBehavior(t *testing.T) {
	tests := []struct {
		name         string
		statuses     []int
		wantAttempts int32
		wantErr      bool
	}{
		{"5xx succeeds on the second attempt", []int{http.StatusInternalServerError, http.StatusOK}, 2, false},
		{"persistent 5xx exhausts all 3 attempts", []int{http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError}, 3, true},
		{"4xx fails after exactly one attempt, no retry", []int{http.StatusBadRequest}, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				i := calls.Add(1) - 1
				if int(i) >= len(tt.statuses) {
					t.Errorf("unexpected extra request #%d", i+1)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				status := tt.statuses[i]
				if status == http.StatusOK {
					writeJSON(w, status, httpWireResponse{ScoreA: 0.7, ScoreB: 0.3, Winner: WinnerA, Reason: "ok"})
					return
				}
				writeJSON(w, status, httpErrorBody{})
			}))
			defer server.Close()

			hj := &HTTPJudge{baseURL: server.URL, timeout: time.Second, client: server.Client(), retryBase: time.Millisecond}
			_, err := hj.Score(context.Background(), Request{Prompt: "p", ImageA: []byte("a"), ImageB: []byte("b")})
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got := calls.Load(); got != tt.wantAttempts {
				t.Errorf("attempts = %d, want %d", got, tt.wantAttempts)
			}
		})
	}
}

// TestHTTPJudge_Score_RejectsContractViolations checks that a 200 body
// violating §2 — out of range, a bad winner, an over-long reason — is
// rejected as ErrInvalidResult and never retried: the service is pure, so a
// same-content retry would only earn the same broken body back.
func TestHTTPJudge_Score_RejectsContractViolations(t *testing.T) {
	longReason := strings.Repeat("x", maxReasonLen+1)
	tests := []struct {
		name string
		resp httpWireResponse
	}{
		{"score out of range", httpWireResponse{ScoreA: 1.5, ScoreB: 0.5, Winner: WinnerA, Reason: "ok"}},
		{"invalid winner", httpWireResponse{ScoreA: 0.5, ScoreB: 0.5, Winner: "C", Reason: "ok"}},
		{"reason too long", httpWireResponse{ScoreA: 0.5, ScoreB: 0.5, Winner: WinnerTie, Reason: longReason}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				writeJSON(w, http.StatusOK, tt.resp)
			}))
			defer server.Close()

			hj := NewHTTPJudge(server.URL, time.Second)
			_, err := hj.Score(context.Background(), Request{Prompt: "p", ImageA: []byte("a"), ImageB: []byte("b")})
			if err == nil {
				t.Fatal("expected an error for a contract-violating 200")
			}
			if !errors.Is(err, ErrInvalidResult) {
				t.Errorf("err = %v, want it to wrap ErrInvalidResult", err)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("attempts = %d, want 1 (a contract violation must not be retried)", got)
			}
		})
	}
}

// TestHTTPJudge_Score_AttemptTimeout proves the timeout is applied PER ATTEMPT
// (internal/platform/config's JudgeTimeout doc comment): a server slower than
// the timeout makes every one of the 3 attempts fail fast, well short of the
// server's own delay, rather than one attempt eating the whole budget.
func TestHTTPJudge_Score_AttemptTimeout(t *testing.T) {
	// The delay only needs to comfortably exceed the 20ms per-attempt timeout
	// below — it is deliberately NOT seconds-long, because a canceled client
	// request does not reliably interrupt an in-flight handler that isn't
	// itself watching r.Context(), and server.Close() (deferred) blocks until
	// every handler invocation returns.
	const serverDelay = 300 * time.Millisecond

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case <-time.After(serverDelay):
		case <-r.Context().Done():
		}
	}))
	defer server.Close()

	hj := &HTTPJudge{baseURL: server.URL, timeout: 20 * time.Millisecond, client: server.Client(), retryBase: time.Millisecond}
	start := time.Now()
	_, err := hj.Score(context.Background(), Request{Prompt: "p", ImageA: []byte("a"), ImageB: []byte("b")})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error when every attempt exceeds its per-attempt timeout")
	}
	if elapsed > serverDelay {
		t.Errorf("Score took %s, want well under the server's %s delay (the per-attempt timeout should cut every attempt short)", elapsed, serverDelay)
	}
	if got := calls.Load(); got != httpMaxAttempts {
		t.Errorf("attempts = %d, want %d", got, httpMaxAttempts)
	}
}

// TestHTTPJudge_Score_CallerCancellationWins proves the OUTER ctx — not just
// our own per-attempt timeout — aborts promptly and is never retried: the
// per-attempt timeout here is deliberately much larger than the ctx, so only
// the caller's own cancellation can be what stops this.
func TestHTTPJudge_Score_CallerCancellationWins(t *testing.T) {
	// Comfortably longer than the 30ms outer ctx below but still short, for
	// the same server.Close()-blocks-on-lingering-handlers reason as the
	// per-attempt timeout test above.
	const serverDelay = 300 * time.Millisecond

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case <-time.After(serverDelay):
		case <-r.Context().Done():
		}
	}))
	defer server.Close()

	// timeout is deliberately much larger than the ctx below, so only the
	// caller's own ctx can be what stops this.
	hj := &HTTPJudge{baseURL: server.URL, timeout: time.Second, client: server.Client(), retryBase: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := hj.Score(ctx, Request{Prompt: "p", ImageA: []byte("a"), ImageB: []byte("b")})
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want it to wrap context.DeadlineExceeded", err)
	}
	if elapsed > serverDelay {
		t.Errorf("Score took %s, want it to abort promptly on the caller's own ctx", elapsed)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("attempts = %d, want exactly 1 (no retry after the caller's own cancellation)", got)
	}
}

// TestHTTPJudge_Score_IdempotencyKeyStableAcrossRetries locks in §6's "the
// same key for retries of the same scoring": every attempt this call makes
// must carry the identical Idempotency-Key.
func TestHTTPJudge_Score_IdempotencyKeyStableAcrossRetries(t *testing.T) {
	var mu sync.Mutex
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		n := len(keys)
		mu.Unlock()
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, httpWireResponse{ScoreA: 0.5, ScoreB: 0.5, Winner: WinnerTie, Reason: "ok"})
	}))
	defer server.Close()

	hj := &HTTPJudge{baseURL: server.URL, timeout: time.Second, client: server.Client(), retryBase: time.Millisecond}
	_, err := hj.Score(context.Background(), Request{Prompt: "p", ImageA: []byte("a"), ImageB: []byte("b")})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(keys) != 3 {
		t.Fatalf("got %d requests, want 3", len(keys))
	}
	for i, k := range keys {
		if k == "" {
			t.Errorf("request %d: empty Idempotency-Key", i)
		}
		if k != keys[0] {
			t.Errorf("request %d key = %q, want %q (same key as the first attempt)", i, k, keys[0])
		}
	}
}
