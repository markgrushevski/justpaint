package game

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

// maxMatchBodyBytes bounds the tiny create-match body ({mode?}).
const maxMatchBodyBytes = 4 << 10 // 4 KiB

// Handler is the HTTP layer for the async duel (docs/API.md §8).
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Routes registers the match routes; protect is the auth middleware (every route
// requires a session).
func (h *Handler) Routes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("POST /api/matches", protect(http.HandlerFunc(h.Create)))
	mux.Handle("GET /api/matches/{id}", protect(http.HandlerFunc(h.Get)))
}

// --- DTOs ---

type createMatchRequest struct {
	Mode string `json:"mode"`
}

type promptDTO struct {
	ID string `json:"id"`
	// Text is null until the match leaves `open` — a player must not see the
	// prompt while waiting alone, or they could pre-draw (docs/GAME.md §5).
	Text *string `json:"text"`
}

type canvasDTO struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type playerDTO struct {
	UserID      string  `json:"userId"`
	DisplayName *string `json:"displayName"`
	Submitted   bool    `json:"submitted"`
	// DrawingID is a player's own once submitted; the opponent's appears only on
	// the `done` result (docs/GAME.md §4.2). Omitted while redacted.
	DrawingID *string `json:"drawingId,omitempty"`
}

type matchDTO struct {
	ID        string      `json:"id"`
	Mode      string      `json:"mode"`
	Status    string      `json:"status"`
	Prompt    promptDTO   `json:"prompt"`
	Canvas    canvasDTO   `json:"canvas"`
	Players   []playerDTO `json:"players"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

type matchEnvelope struct {
	Match matchDTO `json:"match"`
}

// buildMatchDTO renders a MatchView for one viewer, applying the two visibility
// rules (docs/GAME.md §4.2, §5): the prompt text is hidden until the match leaves
// `open`, and a player sees only their own drawingId until the match is `done`.
// Pure — no DB, no HTTP — so the redaction is table-tested directly.
func buildMatchDTO(v MatchView, viewerID string) matchDTO {
	prompt := promptDTO{ID: v.PromptID}
	if v.Status != statusOpen {
		text := v.PromptText
		prompt.Text = &text
	}

	players := make([]playerDTO, len(v.Players))
	for i, p := range v.Players {
		dto := playerDTO{
			UserID:      p.UserID,
			DisplayName: p.DisplayName,
			Submitted:   p.DrawingID != nil,
		}
		if p.UserID == viewerID || v.Status == statusDone {
			dto.DrawingID = p.DrawingID
		}
		players[i] = dto
	}

	return matchDTO{
		ID:        v.ID,
		Mode:      v.Mode,
		Status:    v.Status,
		Prompt:    prompt,
		Canvas:    canvasDTO{Width: GameCanvasSize, Height: GameCanvasSize},
		Players:   players,
		CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt,
	}
}

// --- handlers ---

// Create: POST /api/matches — create or auto-join an async match (auth: required).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context()) // RequireAuth guarantees presence

	// The body is optional ({mode?}); an empty body (io.EOF) means "defaults".
	var req createMatchRequest
	if err := web.DecodeJSON(w, r, &req, maxMatchBodyBytes); err != nil && !errors.Is(err, io.EOF) {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "invalid request body")
		return
	}
	// v1 accepts only "async" (the default). The value is validated here but not
	// threaded into CreateOrJoin: creation relies on the matches.mode column
	// default ('async') and auto-join only considers async matches, so mode is
	// intentionally inert until live mode lands (docs/GAME.md §9).
	mode := req.Mode
	if mode == "" {
		mode = modeAsync
	}
	if mode != modeAsync {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "mode must be async")
		return
	}

	view, err := h.svc.CreateOrJoin(r.Context(), uid)
	if err != nil {
		if errors.Is(err, ErrNoPrompts) {
			h.logger.Error("create match: no active prompts — run the seed migration (00002)")
		} else {
			h.logger.Error("create match", "err", err)
		}
		web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		return
	}
	web.JSON(w, http.StatusCreated, matchEnvelope{Match: buildMatchDTO(view, uid)})
}

// Get: GET /api/matches/{id} — redacted match state (auth: required; must be a
// player, else hidden 404).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context())

	// A non-UUID id can never name a row; hide it as 404 (uniform with foreign
	// ids) rather than letting it reach the ::uuid cast as a 500.
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
		return
	}

	view, err := h.svc.Get(r.Context(), uid, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
			return
		}
		h.logger.Error("get match", "err", err)
		web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		return
	}
	web.JSON(w, http.StatusOK, matchEnvelope{Match: buildMatchDTO(view, uid)})
}
