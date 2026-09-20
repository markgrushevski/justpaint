package guess

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/document"
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
	// nothing the client rasterizes is ever what we send onward (see Service.Guess).
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

	// document.ParseAndValidate, NOT game.ValidateSubmission — the one line in this
	// file worth stopping at, because the copy-paste is sitting right there in
	// internal/practice, which does use the duel's validator.
	//
	// The duel's validator adds a square GameCanvasSize (1080²) rule on top of the
	// document contract, and that rule belongs to the DUEL: two players are compared
	// against each other, so they must draw on the same canvas. A free-draw canvas is
	// whatever the player made it, any size the format allows, and importing the
	// duel's rule here would 400 exactly the drawings this feature exists to look at.
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
		switch {
		// Both budget refusals are 429s and neither is the player's fault in the same
		// way; the copy for both lives in one place (aibudget.WriteRefusal), because
		// deciding what a refusal discloses is one decision, not one per feature.
		case aibudget.WriteRefusal(w, err):
			// handled — the response is already written
		default:
			h.fail(w, err)
		}
		return
	}

	// Always an array on the wire, never null. The client renders 0-2 runner-ups;
	// an absent list and an empty one are the same fact, and it should not have to
	// know two spellings of it.
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

// fail maps a server-side failure to 500. ErrNotConfigured gets a message that
// names the cause instead of the usual opaque "internal error": it is a
// deployment fact, not a secret, and "the guess is unavailable" with no reason is
// how a misconfiguration survives for a week.
func (h *Handler) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotConfigured) {
		h.logger.Error("guess: no guesser is configured — JUDGE_MODE=http scores duels only", "err", err)
		web.Error(w, http.StatusInternalServerError, web.CodeInternal,
			"the AI guess is not available on this server: the configured judge cannot look at a single drawing")
		return
	}
	h.logger.Error("guess drawing", "err", err)
	web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
}
