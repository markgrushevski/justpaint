package guess

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

// maxGuessBodyBytes is the 8 MB document cap shared with drawings, duel
// submissions and practice runs (docs/API.md §6) — the same artefact, so the same
// ceiling.
const maxGuessBodyBytes = 8 << 20 // 8 MiB

// Handler is the HTTP layer for the /draw guess button. One route, behind
// RequireAuth: a call is billed to a player's daily allowance, so there is no
// anonymous path to it.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler builds the guess HTTP handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Routes registers the guess route; protect is the auth middleware.
func (h *Handler) Routes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("POST /api/guess", protect(http.HandlerFunc(h.Guess)))
}

// --- DTOs ---

type guessRequest struct {
	Document json.RawMessage `json:"document"`
	// A client thumbnail may ride along but is advisory only and ignored here —
	// nothing the client rasterizes is ever what we send onward.
}

type guessDTO struct {
	Label        string   `json:"label"`
	Confidence   float64  `json:"confidence"`
	Alternatives []string `json:"alternatives"`
}

type guessEnvelope struct {
	Guess guessDTO `json:"guess"`
}

// --- handlers ---

// Guess: POST /api/guess — say what the caller's free-draw canvas is (auth:
// required). Returns 200 with the answer, never 202: there is nobody to wait for,
// so the request that asks is the request that learns.
func (h *Handler) Guess(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context()) // RequireAuth guarantees presence

	var req guessRequest
	// Lax: the document (plus an advisory thumbnail) must stay forward-compatible,
	// exactly as on every other document-bearing route (docs/API.md §1).
	if err := web.DecodeJSONLax(w, r, &req, maxGuessBodyBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			web.Error(w, http.StatusRequestEntityTooLarge, web.CodeDocumentTooLarge, "document exceeds the size limit")
		} else {
			web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "invalid request body")
		}
		return
	}

	// document.ParseAndValidate, NOT game.ValidateSubmission (which internal/practice
	// uses): the duel's square GameCanvasSize rule belongs to the duel, where two
	// players are compared on the same canvas. A free-draw canvas is any size the
	// format allows, and importing that rule here would 400 exactly the drawings
	// this feature exists to look at.
	doc, err := document.ParseAndValidate(req.Document)
	if err != nil {
		msg := "invalid document"
		var ve *document.ValidationError
		if errors.As(err, &ve) {
			msg = ve.Msg
		}
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, msg)
		return
	}

	view, err := h.svc.Guess(r.Context(), uid, doc)
	if err != nil {
		// aibudget.WriteRefusal owns the copy for both 429 cases and writes the
		// response itself, so it is branched on here rather than inside a switch
		// predicate.
		if aibudget.WriteRefusal(w, err) {
			return
		}
		// The provider's own quota ran out, learned from a 429 on the wire — the same
		// news as our own ceiling, so it gets the same refusal rather than a 500 that
		// invites a retry that cannot succeed until the provider's window resets.
		if errors.Is(err, judge.ErrQuotaExhausted) {
			h.logger.Error("guess: provider quota exhausted — every guess fails until it resets", "err", err)
			aibudget.WriteRefusal(w, aibudget.ErrGlobalSpent)
			return
		}
		h.fail(w, err)
		return
	}

	// Always [] on the wire, never null: the client renders 0-2 runner-ups and
	// should not have to know two spellings of "none".
	alternatives := view.Alternatives
	if alternatives == nil {
		alternatives = []string{}
	}
	web.JSON(w, http.StatusOK, guessEnvelope{Guess: guessDTO{
		Label:        view.Label,
		Confidence:   view.Confidence,
		Alternatives: alternatives,
	}})
}

// fail maps a server-side failure to 500. ErrNotConfigured names the cause
// instead of the usual opaque message — a misconfiguration is a deployment fact,
// not a secret worth hiding.
func (h *Handler) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotConfigured) {
		h.logger.Error("guess: no guesser is configured — JUDGE_MODE=http scores duels only", "err", err)
		web.Error(w, http.StatusInternalServerError, web.CodeInternal,
			"the AI guess is turned off on this server")
		return
	}
	h.logger.Error("guess drawing", "err", err)
	web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
}
