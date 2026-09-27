package judge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const httpScorePath = "/v1/score"

// httpContractVersion is sent on every call; v1 does not require the judge to echo it.
const httpContractVersion = "1"

const httpMaxAttempts = 3 // 1 try + 2 retries (JUDGE.md §7)

// httpRetryBase is the first backoff step, doubling per retry (250ms, 500ms).
// game.JudgePassBudget is sized from this envelope.
const httpRetryBase = 250 * time.Millisecond

// httpMaxResponseBytes stops a misbehaving endpoint; a real result is a few hundred bytes.
const httpMaxResponseBytes = 1 << 20 // 1 MiB

// HTTPJudge implements Judge over the external service's wire contract (JUDGE.md §6,
// retries §7). It is safe for concurrent use. Images travel inline only: the URL
// delivery mode needs object storage this project doesn't have.
type HTTPJudge struct {
	baseURL   string
	timeout   time.Duration
	client    *http.Client
	retryBase time.Duration // tests set a tiny value
}

// NewHTTPJudge builds a client for the judge at baseURL. timeout bounds one attempt,
// not the whole call, so a slow attempt cannot eat the next retry's time.
func NewHTTPJudge(baseURL string, timeout time.Duration) *HTTPJudge {
	return &HTTPJudge{
		baseURL:   strings.TrimRight(baseURL, "/"),
		timeout:   timeout,
		client:    &http.Client{},
		retryBase: httpRetryBase,
	}
}

var _ Judge = (*HTTPJudge)(nil)

// httpWireRequest is the JUDGE.md §6 body; encoding/json base64-encodes the []byte
// images, which is the inline transport.
type httpWireRequest struct {
	Prompt string `json:"prompt"`
	ImageA []byte `json:"imageA"`
	ImageB []byte `json:"imageB"`
}

// httpWireResponse is the JudgeResult on the wire. encoding/json ignores unknown
// fields, which is all the JUDGE.md §10 forward compatibility asks for.
type httpWireResponse struct {
	ScoreA float64 `json:"scoreA"`
	ScoreB float64 `json:"scoreB"`
	Winner string  `json:"winner"`
	Reason string  `json:"reason"`
}

// httpErrorBody is the judge's non-2xx error shape. Its code space is the judge's own,
// so it only feeds error lines and is never branched on.
type httpErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Score implements Judge. It retries connection errors, timeouts and 5xx with doubling
// backoff, never a 4xx or an invalid 200; scoring is pure, so a retry is safe.
func (h *HTTPJudge) Score(ctx context.Context, req Request) (Result, error) {
	body, err := json.Marshal(httpWireRequest{Prompt: req.Prompt, ImageA: req.ImageA, ImageB: req.ImageB})
	if err != nil {
		return Result{}, fmt.Errorf("judge: http: encode request: %w", err)
	}
	key := idempotencyKey(req) // the same key for every attempt (JUDGE.md §6)

	var lastErr error
	for attempt := 1; attempt <= httpMaxAttempts; attempt++ {
		if attempt > 1 {
			if err := httpBackoff(ctx, h.retryBase, attempt); err != nil {
				return Result{}, fmt.Errorf("judge: http: retry aborted: %w (last failure: %v)", err, lastErr)
			}
		}
		result, retryable, err := h.attempt(ctx, body, key)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retryable {
			return Result{}, err
		}
		if ctx.Err() != nil {
			return Result{}, fmt.Errorf("judge: http: call aborted: %w (last failure: %v)", ctx.Err(), lastErr)
		}
	}
	return Result{}, fmt.Errorf("judge: http: failed after %d attempts: %w", httpMaxAttempts, lastErr)
}

// attempt makes one round trip; the bool says whether a non-nil err is worth a retry.
// A cancelled caller is Score's to catch.
func (h *HTTPJudge) attempt(ctx context.Context, body []byte, idemKey string) (Result, bool, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, h.baseURL+httpScorePath, bytes.NewReader(body))
	if err != nil {
		return Result{}, false, fmt.Errorf("judge: http: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Judge-Contract-Version", httpContractVersion)
	httpReq.Header.Set("Idempotency-Key", idemKey)

	resp, err := h.client.Do(httpReq)
	if err != nil {
		// DNS, TLS, a reset or the per-attempt deadline: all transient.
		return Result{}, true, fmt.Errorf("judge: http: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, httpMaxResponseBytes))
	if err != nil {
		return Result{}, true, fmt.Errorf("judge: http: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Only 5xx is transient; a 4xx or anything else means our request is wrong.
		return Result{}, resp.StatusCode >= 500, fmt.Errorf("judge: http: service %s: %s", resp.Status, summarizeError(respBody))
	}

	var wire httpWireResponse
	if err := json.Unmarshal(respBody, &wire); err != nil {
		return Result{}, false, fmt.Errorf("judge: http: decode 200 body: %w", err)
	}
	result := Result{ScoreA: wire.ScoreA, ScoreB: wire.ScoreB, Winner: wire.Winner, Reason: wire.Reason}
	if err := result.Validate(); err != nil {
		// A contract violation; the judge is pure, so a retry returns the same body.
		return Result{}, false, fmt.Errorf("judge: http: %w", err)
	}
	return result, false, nil
}

// httpBackoff waits retryBase * 2^(attempt-2), cancellably.
func httpBackoff(ctx context.Context, retryBase time.Duration, attempt int) error {
	delay := retryBase << (attempt - 2)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// idempotencyKey hashes the request content, since Request carries no match id. Each
// part is hashed separately so "ab"+"c" and "a"+"bc" cannot collide. A stuck-judging
// re-fire reuses the key, which is safe because scoring is pure.
func idempotencyKey(req Request) string {
	hp := sha256.Sum256([]byte(req.Prompt))
	ha := sha256.Sum256(req.ImageA)
	hb := sha256.Sum256(req.ImageB)
	combined := sha256.New()
	combined.Write(hp[:])
	combined.Write(ha[:])
	combined.Write(hb[:])
	return hex.EncodeToString(combined.Sum(nil))
}

// summarizeError renders a non-2xx body for an error line: the httpErrorBody shape,
// else the raw body, truncated.
func summarizeError(body []byte) string {
	var e httpErrorBody
	if err := json.Unmarshal(body, &e); err == nil && e.Error.Code != "" {
		return fmt.Sprintf("%s: %s", e.Error.Code, e.Error.Message)
	}
	const maxRaw = 200
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return "(empty body)"
	}
	if len(raw) > maxRaw {
		raw = raw[:maxRaw] + "…"
	}
	return raw
}
