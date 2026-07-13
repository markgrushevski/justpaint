package assist

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

// maxAssistBodyBytes caps the assist request body: a prompt plus the MINIMAL doc
// summary (canvas + layer inventory, never the full document), so 64 KiB is
// generous (docs/DESIGN-ASSIST-PHASE-A.md §2.3).
const maxAssistBodyBytes = 64 << 10 // 64 KiB

// Handler is the HTTP layer for AI assist (docs/ASSIST.md §3, docs/API.md).
type Handler struct {
	assist  Assist
	limiter *RateLimiter
	logger  *slog.Logger
}

// NewHandler builds the assist HTTP handler over an Assist impl and a per-user
// rate limiter.
func NewHandler(a Assist, limiter *RateLimiter, logger *slog.Logger) *Handler {
	return &Handler{assist: a, limiter: limiter, logger: logger}
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
