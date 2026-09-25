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

// httpScorePath is appended to baseURL for every call (JUDGE.md §6).
const httpScorePath = "/v1/score"

// httpContractVersion is the wire version this client speaks (JUDGE.md §6,
// §10). The judge SHOULD echo it back on responses; v1 has nothing yet to
// negotiate on a mismatch, so we don't require that.
const httpContractVersion = "1"

// httpMaxAttempts = 1 initial try + 2 retries (JUDGE.md §7).
const httpMaxAttempts = 3

// httpRetryBase is the first backoff step; it doubles per retry (250ms, then
// 500ms — the two sleeps that fit between the 3 attempts §7 allows). §7 pins
// the retry COUNT, not a duration; this default stays comfortably inside
// game.JudgePassBudget, which covers the whole judging pass (two authoritative
// renders plus the score call — internal/game/service.go). That budget is sized
// FROM this retry envelope, not the other way round: it was a flat 30s, which
// silently left no room for the third attempt. Same policy, and the same field
// name, as GeminiJudge's retryBase.
const httpRetryBase = 250 * time.Millisecond

// httpMaxResponseBytes bounds how much of a response body we'll read. A real
// JudgeResult is at most a few hundred bytes (§2 caps reason at 500 chars);
// this only stops a misbehaving endpoint from streaming unbounded data into
// memory.
const httpMaxResponseBytes = 1 << 20 // 1 MiB

// HTTPJudge is the transport to the external ML judge's real service
// (docs/JUDGE.md §7), implementing Judge over the wire contract §6 pins. It is
// stateless and safe for concurrent use — the game module runs up to
// JudgeConcurrency judging passes at once (internal/game/service.go).
//
// Images travel INLINE ONLY. Request.ImageA/ImageB stay typed []byte all the
// way to the wire purely so encoding/json's built-in rule — a []byte field
// marshals as a base64 string — does the encoding for us; there is no custom
// marshaling and no `data:` URI prefix. §6 also describes a URL delivery mode,
// meant to keep request bodies tiny once object storage exists; this project
// deliberately has none (docs/ROADMAP.md Phase 4 defers it, and the /play
// opponent-canvas reveal was built as a membership-gated endpoint specifically
// to avoid needing it — docs/DECISIONS.md). So JUDGE_IMAGE_MODE is NOT
// implemented here: a config knob with exactly one legal value is worse than
// no knob at all. Add url mode — and the knob — together, when object storage
// lands.
type HTTPJudge struct {
	baseURL string
	timeout time.Duration
	client  *http.Client
	// retryBase is the first backoff step; NewHTTPJudge sets httpRetryBase.
	// Tests in this package construct HTTPJudge directly with a tiny value so
	// the retry paths don't slow down the suite.
	retryBase time.Duration
}

// NewHTTPJudge builds a client for the external ML judge's service at baseURL (a
// trailing slash is tolerated and trimmed, so callers can't accidentally send
// a doubled slash before /v1/score). timeout bounds ONE attempt, retries
// excluded: internal/platform/config's JudgeTimeout doc comment reads §7's
// "per-call deadline" as per-attempt, so one slow attempt doesn't eat into the
// next retry's own budget.
func NewHTTPJudge(baseURL string, timeout time.Duration) *HTTPJudge {
	return &HTTPJudge{
		baseURL:   strings.TrimRight(baseURL, "/"),
		timeout:   timeout,
		client:    &http.Client{},
		retryBase: httpRetryBase,
	}
}

var _ Judge = (*HTTPJudge)(nil)

// httpWireRequest is the §6 request body. Prompt aside, the whole reason this
// type exists separately from Request is the []byte fields: encoding/json
// base64-encodes a []byte automatically, which is exactly inline transport.
type httpWireRequest struct {
	Prompt string `json:"prompt"`
	ImageA []byte `json:"imageA"`
	ImageB []byte `json:"imageB"`
}

// httpWireResponse is the §2 JudgeResult as it appears on the wire. Unknown
// extra fields are silently ignored by encoding/json already, which is all
// the §10 forward-compat rule ("tolerate extra response fields") requires.
type httpWireResponse struct {
	ScoreA float64 `json:"scoreA"`
	ScoreB float64 `json:"scoreB"`
	Winner string  `json:"winner"`
	Reason string  `json:"reason"`
}

// httpErrorBody mirrors the {error:{code,message}} shape §6 promises on a
// non-2xx. That code space belongs to the external ML judge, independent of
// API.md §3's closed set (§6's note) — we only read it to make an error/log line
// readable, never to branch on.
type httpErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Score implements Judge. The retry policy mirrors JUDGE.md §7: up to 2
// retries on connection errors, timeouts and 5xx, with doubling backoff; never
// on a 4xx or a contract-violating 200 — neither is a fault another attempt
// could fix, and scoring being pure is what makes the retries that DO happen
// safe.
func (h *HTTPJudge) Score(ctx context.Context, req Request) (Result, error) {
	body, err := json.Marshal(httpWireRequest{Prompt: req.Prompt, ImageA: req.ImageA, ImageB: req.ImageB})
	if err != nil {
		return Result{}, fmt.Errorf("judge: http: encode request: %w", err)
	}
	// One key for every attempt below — "the same key for retries of the same
	// scoring" (§6). See idempotencyKey for why it's a content hash rather
	// than a caller-supplied id.
	key := idempotencyKey(req)

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
		// The caller giving up outranks our retry budget: a cancelled parent
		// means nobody is waiting for this verdict any more.
		if ctx.Err() != nil {
			return Result{}, fmt.Errorf("judge: http: call aborted: %w (last failure: %v)", ctx.Err(), lastErr)
		}
	}
	return Result{}, fmt.Errorf("judge: http: failed after %d attempts: %w", httpMaxAttempts, lastErr)
}

// attempt makes one HTTP round trip and classifies the failure, if any. The
// bool return is meaningful only when err != nil: true means worth retrying.
// It does not itself special-case a cancelled caller ctx — any Do failure is
// reported as retryable, and Score's own ctx.Err() check (made once per
// iteration, right after seeing retryable=true) is what stops a cancelled
// caller from actually being retried.
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
		// DNS, TLS, connection reset, or our own per-attempt deadline — all §7
		// transient.
		return Result{}, true, fmt.Errorf("judge: http: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, httpMaxResponseBytes))
	if err != nil {
		return Result{}, true, fmt.Errorf("judge: http: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// 5xx is transient; a 4xx means OUR request is wrong, and any other
		// unexpected status is treated the same way — neither is worth
		// hammering (§7).
		return Result{}, resp.StatusCode >= 500, fmt.Errorf("judge: http: service %s: %s", resp.Status, summarizeError(respBody))
	}

	var wire httpWireResponse
	if err := json.Unmarshal(respBody, &wire); err != nil {
		return Result{}, false, fmt.Errorf("judge: http: decode 200 body: %w", err)
	}
	result := Result{ScoreA: wire.ScoreA, ScoreB: wire.ScoreB, Winner: wire.Winner, Reason: wire.Reason}
	if err := result.Validate(); err != nil {
		// A 200 that fails §2 validation is a CONTRACT VIOLATION, not a
		// verdict, and not worth a retry either: the external ML judge's service is
		// pure, so a same-content retry earns the same broken body back.
		// Validate's error already wraps ErrInvalidResult.
		return Result{}, false, fmt.Errorf("judge: http: %w", err)
	}
	return result, false, nil
}

// httpBackoff waits retryBase * 2^(attempt-2) — 250ms, then 500ms by default —
// while staying cancellable. Mirrors GeminiJudge's backoff: JUDGE.md §7 is the
// one retry policy both implementations follow.
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

// idempotencyKey derives a stable key from the request content (§6: "the same
// key for retries of the same scoring"). Request carries no match/submit id —
// the judge package knows nothing of matches, game owns that mapping (§4) —
// so content is the only stable handle available here. Hashing prompt and
// each image separately before combining avoids the ambiguity of
// concatenating variable-length fields directly (prompt "ab" + imageA "c"
// would otherwise collide with prompt "a" + imageA "bc"). A pleasant side
// effect: byte-identical rescoring — e.g. the stuck-judging sweep re-running
// the same submissions (docs/GAME.md §4.1) — reuses the same key too, which is
// exactly what "safe to ignore since scoring is pure" (§7) is describing.
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

// summarizeError renders a non-2xx body for an error/log line. It tries the
// {error:{code,message}} shape §6 promises and falls back to a truncated raw
// body when the response doesn't match it.
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
