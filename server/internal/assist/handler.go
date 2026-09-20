package assist

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

// maxAssistBodyBytes caps the assist request body: a prompt plus the MINIMAL doc
// summary (canvas + layer inventory, never the full document), so 64 KiB is
// generous (docs/DESIGN-ASSIST-PHASE-A.md §2.3).
const maxAssistBodyBytes = 64 << 10 // 64 KiB

// maxPromptBytes caps the natural-language prompt. Well under the body cap (which
// also carries the doc summary), it rejects a runaway prompt before it reaches the
// LLM impl — every assist call can cost real API money (docs/ASSIST.md §3.4).
const maxPromptBytes = 8 << 10 // 8 KiB

// BudgetCheck and BudgetSpend are assist's two questions to the daily AI-call
// ceiling (internal/aibudget), held as plain funcs so this package never learns
// its own kind's name or the budget's shape. Nil means unbudgeted.
//
// They answer a DIFFERENT question from the limiter beside them. The token bucket
// bounds the RATE — how fast one user may ask — and lives in this process, so the
// host resets it on every deploy and every wake from idle. The budget bounds the
// daily QUOTA, lives in Postgres, and therefore actually holds. Assist has had
// only the first since Phase A, which means its ceiling has never survived a
// restart; layering both on one endpoint is the pattern docs/API.md §3.1 already
// documents for POST /api/matches.
type (
	BudgetCheck func(ctx context.Context, userID string) error
	BudgetSpend func(ctx context.Context, userIDs ...string) error
)

// Handler is the HTTP layer for AI assist (docs/ASSIST.md §3, docs/API.md).
type Handler struct {
	assist  Assist
	limiter *RateLimiter
	budget  BudgetCheck
	spend   BudgetSpend
	logger  *slog.Logger
}

// NewHandler builds the assist HTTP handler over an Assist impl, a per-user rate
// limiter and the daily AI-call budget ports.
func NewHandler(a Assist, limiter *RateLimiter, budget BudgetCheck, spend BudgetSpend, logger *slog.Logger) *Handler {
	return &Handler{assist: a, limiter: limiter, budget: budget, spend: spend, logger: logger}
}

// Routes registers the assist route; protect is the auth middleware — the route
// requires a session exactly like every other write route (docs/ASSIST.md §3.1).
func (h *Handler) Routes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("POST /api/assist/ops", protect(http.HandlerFunc(h.GenerateOps)))
}

// GenerateOps: POST /api/assist/ops — turn a prompt into a validated op batch
// (auth: required). Rate-limited per user (each call can cost real API money —
// docs/ASSIST.md §3.4). The returned ops are re-validated server-side, so the
// client never receives an unvalidated batch (trust boundary).
func (h *Handler) GenerateOps(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context()) // RequireAuth guarantees presence

	// Rate limit FIRST — before decoding or spending an LLM call. On exceed, set
	// Retry-After BEFORE web.Error: web.Error → JSON → w.WriteHeader, and headers
	// set after WriteHeader are silently dropped by net/http
	// (docs/DESIGN-ASSIST-PHASE-A.md §2.3 gotcha).
	if !h.limiter.Allow(uid) {
		secs := int(h.limiter.RetryAfter().Seconds())
		w.Header().Set("Retry-After", strconv.Itoa(max(1, secs)))
		web.Error(w, http.StatusTooManyRequests, web.CodeRateLimited, "too many assist requests")
		return
	}

	var req Request
	if err := web.DecodeJSON(w, r, &req, maxAssistBodyBytes); err != nil {
		// Malformed JSON, unknown fields, or an over-cap body all fold into the one
		// client error path (docs/API.md §1 strict decode; no 422 anywhere).
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "invalid request body")
		return
	}

	// Guard the prompt BEFORE spending an LLM call: an empty/whitespace-only prompt
	// is nothing to draw, and an over-long one is rejected up front (defense against
	// a runaway prompt sneaking under the 64 KiB body cap).
	if strings.TrimSpace(req.Prompt) == "" {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "prompt must not be empty")
		return
	}
	if len(req.Prompt) > maxPromptBytes {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "prompt is too long")
		return
	}

	// The daily ceiling, last of the guards and immediately before the only
	// expensive line: everything above is free to re-run, and the whole point of a
	// quota is to refuse BEFORE the call, never after paying for it.
	if h.budget != nil {
		if err := h.budget(r.Context(), uid); err != nil {
			if aibudget.WriteRefusal(w, err) {
				return
			}
			h.logger.Error("assist budget check", "err", err)
			web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
			return
		}
	}
	// Recorded BEFORE the call, never after it: a call that fails still spent the
	// provider's quota, and a failure the budget cannot see is exactly what a
	// broken impl drains it through (the rule practice states in full).
	if h.spend != nil {
		if err := h.spend(r.Context(), uid); err != nil {
			h.logger.Error("assist budget spend", "err", err)
			web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
			return
		}
	}

	res, err := h.assist.GenerateOps(r.Context(), req)
	if err != nil {
		// Retry-exhaustion is a client-visible outcome, not a server fault:
		// 400 validation_failed, never 422 (docs/API.md:68). Anything else is internal.
		if errors.Is(err, ErrInvalidBatch) {
			web.Error(w, http.StatusBadRequest, web.CodeValidationFailed,
				"could not generate a valid drawing for that prompt")
			return
		}
		h.logger.Error("assist generate", "err", err)
		web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		return
	}

	// Defense-in-depth: re-validate against the request summary before the batch
	// reaches the client. An impl bug or a future real model must never hand the
	// client unvalidated ops — the client applies them as editor commands.
	if err := document.ValidateOpBatch(res.Ops, req.DocSummary); err != nil {
		msg := "generated ops failed validation"
		var ve *document.ValidationError
		if errors.As(err, &ve) {
			msg = ve.Msg
		}
		h.logger.Warn("assist ops failed server validation", "err", err)
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, msg)
		return
	}

	web.JSON(w, http.StatusOK, res)
}
