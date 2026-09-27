package assist

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

// maxAssistBodyBytes caps the assist request body: a prompt plus the minimal
// doc summary (canvas + layer inventory, never the full document), so 64 KiB
// is generous (docs/API.md §10).
const maxAssistBodyBytes = 64 << 10 // 64 KiB

// maxPromptBytes caps the natural-language prompt, well under the body cap,
// rejecting a runaway prompt before it reaches the LLM impl — every assist
// call can cost real API money (docs/ASSIST.md §3.4).
const maxPromptBytes = 8 << 10 // 8 KiB

// ProviderCaller is implemented by an Assist impl whose GenerateOps really
// reaches an external provider and so really spends a quota.
//
// Which impl was asked for and whether it calls anybody are different facts:
// deriving the second from the mode risks billing a ledger row for an impl
// that made no call. The impl is the only thing that knows, so it is what
// gets asked.
type ProviderCaller interface {
	// CallsProvider reports whether GenerateOps performs external API calls.
	CallsProvider() bool
}

// CallsProvider reports whether this impl spends a provider's quota, and so
// whether the composition root should give assist a budget provider at all.
//
// An impl that doesn't implement ProviderCaller counts as making no external
// call — the safe default, since billing an impl that makes no calls charges
// players for nothing, while a real impl that forgets to say so under-counts
// a ceiling that already sits below the provider's own quota.
func CallsProvider(a Assist) bool {
	pc, ok := a.(ProviderCaller)
	return ok && pc.CallsProvider()
}

// The two budget ports are aibudget's own func types, bound to
// aibudget.KindAssist at the composition root: aibudget.Check asks whether
// this player may spend an AI call right now, aibudget.Spend records the one
// they just spent. Nil means unbudgeted.
//
// They answer a different question from the limiter beside them: the token
// bucket bounds the rate and lives in this process (reset on every deploy),
// while the budget bounds the daily quota and lives in Postgres, so it
// actually holds. Layering both on one endpoint mirrors POST /api/matches
// (docs/API.md §3.1).

// writeSlack is how long the response may take to write once the run budget
// is spent.
const writeSlack = 5 * time.Second

// Handler is the HTTP layer for AI assist (docs/ASSIST.md §3, docs/API.md).
type Handler struct {
	assist    Assist
	limiter   *RateLimiter
	budget    aibudget.Check
	spend     aibudget.Spend
	runBudget time.Duration
	logger    *slog.Logger
}

// NewHandler builds the assist HTTP handler over an Assist impl, a per-user rate
// limiter and the daily AI-call budget ports. A positive runBudget bounds the
// generation and keeps the response writable for that long, past the server's
// WriteTimeout; zero leaves both to the server.
func NewHandler(a Assist, limiter *RateLimiter, budget aibudget.Check, spend aibudget.Spend, runBudget time.Duration, logger *slog.Logger) *Handler {
	return &Handler{assist: a, limiter: limiter, budget: budget, spend: spend, runBudget: runBudget, logger: logger}
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

	// Rate limit first, before decoding or spending an LLM call. Set Retry-After
	// before web.Error: web.Error calls WriteHeader, and net/http silently drops
	// headers set after that (docs/NOTES.md "AI assist").
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

	// Guard the prompt before spending an LLM call: empty/whitespace is nothing
	// to draw, and an over-long prompt is rejected here in case it sneaks under
	// the body cap.
	if strings.TrimSpace(req.Prompt) == "" {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "prompt must not be empty")
		return
	}
	if len(req.Prompt) > maxPromptBytes {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "prompt is too long")
		return
	}

	// The daily ceiling is last of the guards, right before the one expensive
	// line: everything above is free to re-run, and a quota must refuse before
	// the call, not after paying for it.
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
	// Recorded before the call, never after: a call that fails still spent the
	// provider's quota. This write is also the tighter of the two per-user
	// gates — it refuses on the count as it stands at the instant of writing,
	// where the check above read one before the body was even decoded — and
	// it answers with the same error, so one branch covers both.
	if h.spend != nil {
		if err := h.spend(r.Context(), uid); err != nil {
			if aibudget.WriteRefusal(w, err) {
				return
			}
			h.logger.Error("assist budget spend", "err", err)
			web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
			return
		}
	}

	ctx := r.Context()
	if h.runBudget > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.runBudget)
		defer cancel()
		// A generation can outlast the server's WriteTimeout, which would drop the
		// response after the call was paid for.
		deadline := time.Now().Add(h.runBudget + writeSlack)
		if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil {
			h.logger.Warn("assist: cannot extend the write deadline", "err", err)
		}
	}

	res, err := h.assist.GenerateOps(ctx, req)
	if err != nil {
		// Retry-exhaustion is a client-visible outcome, not a server fault:
		// 400 validation_failed, never 422 (docs/API.md §3). Anything else is internal.
		if errors.Is(err, ErrInvalidBatch) {
			web.Error(w, http.StatusBadRequest, web.CodeValidationFailed,
				"could not generate a valid drawing for that prompt")
			return
		}
		// The provider ran out before our own ceiling did — the same news for the
		// user, from the other end. Answering 500 would invite a retry that can't
		// succeed until the provider's window rolls, so this gets the same refusal
		// the global ceiling would have written; the cause still reaches the log.
		if errors.Is(err, judge.ErrQuotaExhausted) {
			h.logger.Error("assist: provider quota exhausted — every drawing request fails until it resets", "err", err)
			aibudget.WriteRefusal(w, aibudget.ErrGlobalSpent)
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
