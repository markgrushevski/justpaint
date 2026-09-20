package practice

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/game"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

// maxRunBodyBytes is the 8 MB document cap shared with drawings and duel
// submissions (docs/API.md §6) — the same document, so the same ceiling.
const maxRunBodyBytes = 8 << 20 // 8 MiB

// Handler is the HTTP layer for single-player practice. Two routes, both behind
// RequireAuth: a prompt to draw, and a drawing to score.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler builds the practice HTTP handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Routes registers the practice routes; protect is the auth middleware (a run is
// recorded against a user and spends their share of the judge budget, so there is
// no anonymous path here).
func (h *Handler) Routes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/practice/prompt", protect(http.HandlerFunc(h.Prompt)))
	mux.Handle("POST /api/practice", protect(http.HandlerFunc(h.Run)))
}

// --- DTOs ---

// promptDTO always carries its text, unlike the duel's (which hides it until an
// opponent arrives, so nobody pre-draws). A solo player has nobody to outrun.
type promptDTO struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type promptEnvelope struct {
	Prompt promptDTO `json:"prompt"`
}

type runDTO struct {
	ID       string    `json:"id"`
	Score    float64   `json:"score"`
	Feedback string    `json:"feedback"`
	Prompt   promptDTO `json:"prompt"`
}

type runEnvelope struct {
	Run runDTO `json:"run"`
}

type runRequest struct {
	PromptID string          `json:"promptId"`
	Document json.RawMessage `json:"document"`
	// A client thumbnail may ride along but is advisory only and ignored here.
}

// --- handlers ---

// Prompt: GET /api/practice/prompt — one random active prompt to draw (auth:
// required).
func (h *Handler) Prompt(w http.ResponseWriter, r *http.Request) {
	view, err := h.svc.Prompt(r.Context())
	if err != nil {
		h.fail(w, "practice prompt", err)
		return
	}
	web.JSON(w, http.StatusOK, promptEnvelope{Prompt: promptDTO{ID: view.ID, Text: view.Text}})
}

// Run: POST /api/practice — score the caller's drawing against its prompt (auth:
// required). Returns 200 with the verdict, not 202: unlike a duel there is no
// second player to wait for, so the request that submits is the request that
// learns the score.
func (h *Handler) Run(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context()) // RequireAuth guarantees presence

	var req runRequest
	// Lax: the document (plus an advisory thumbnail) must stay forward-compatible,
	// exactly as on the duel submit path (docs/API.md §1).
	if err := web.DecodeJSONLax(w, r, &req, maxRunBodyBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			web.Error(w, http.StatusRequestEntityTooLarge, web.CodeDocumentTooLarge, "document exceeds the size limit")
		} else {
			web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "invalid request body")
		}
		return
	}

	// A non-UUID id can never name a row; hide it as 404 (uniform with foreign ids)
	// rather than letting it reach the ::uuid cast as a 500.
	if _, err := uuid.Parse(req.PromptID); err != nil {
		web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
		return
	}

	// ONE validator for anything that gets scored — the vector-document contract
	// plus the square game canvas (game.ValidateSubmission). A practice drawing and
	// a duel submission are the same artefact put to the same use.
	doc, err := game.ValidateSubmission(req.Document)
	if err != nil {
		msg := "invalid document"
		var ve *document.ValidationError
		if errors.As(err, &ve) {
			msg = ve.Msg
		}
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, msg)
		return
	}

	view, err := h.svc.Run(r.Context(), uid, req.PromptID, doc)
	if err != nil {
		switch {
		case errors.Is(err, ErrPromptNotFound):
			web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
		// Both budget refusals are 429s and neither is the player's fault in the same
		// way; the copy for both lives in one place now (aibudget.WriteRefusal),
		// because deciding what a refusal discloses is one decision, not one per
		// feature.
		case aibudget.WriteRefusal(w, err):
			// handled — the response is already written
		default:
			h.fail(w, "practice run", err)
		}
		return
	}

	web.JSON(w, http.StatusOK, runEnvelope{Run: runDTO{
		ID:       view.ID,
		Score:    view.Score,
		Feedback: view.Feedback,
		Prompt:   promptDTO{ID: view.Prompt.ID, Text: view.Prompt.Text},
	}})
}

// fail maps a server-side failure to 500. ErrNotConfigured gets a message that
// names the cause instead of the usual opaque "internal error": it is a
// deployment fact, not a secret, and "practice is unavailable" with no reason is
// how a misconfiguration survives for a week.
func (h *Handler) fail(w http.ResponseWriter, what string, err error) {
	if errors.Is(err, ErrNotConfigured) {
		h.logger.Error(what+": practice has no critic — JUDGE_MODE=http scores duels only", "err", err)
		web.Error(w, http.StatusInternalServerError, web.CodeInternal,
			"practice is not available on this server: the configured judge cannot score a single drawing")
		return
	}
	h.logger.Error(what, "err", err)
	web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
}
