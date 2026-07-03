package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

const (
	// maxMatchBodyBytes bounds the tiny create-match body ({mode?}).
	maxMatchBodyBytes = 4 << 10 // 4 KiB
	// maxSubmitBodyBytes is the 8 MB document cap shared with drawings (API.md §6).
	maxSubmitBodyBytes = 8 << 20 // 8 MiB
)

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
	mux.Handle("POST /api/matches/{id}/submit", protect(http.HandlerFunc(h.Submit)))
	mux.Handle("GET /api/matches/{id}/result", protect(http.HandlerFunc(h.Result)))
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

// --- submit ---

type submitYou struct {
	Submitted bool   `json:"submitted"`
	DrawingID string `json:"drawingId"`
}

type submitMatch struct {
	ID     string    `json:"id"`
	Status string    `json:"status"`
	You    submitYou `json:"you"`
}

type submitEnvelope struct {
	Match submitMatch `json:"match"`
}

// Submit: POST /api/matches/{id}/submit — submit the caller's vector document for
// this match (auth: required; must be a player). Returns 202: the submission is
// recorded, the verdict is produced out-of-band (docs/API.md §8.3).
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context())
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
		return
	}

	doc, raw, ok := h.decodeSubmission(w, r)
	if !ok {
		return
	}

	res, err := h.svc.Submit(r.Context(), uid, id, doc, raw)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
		case errors.Is(err, ErrNotPlayer):
			web.Error(w, http.StatusForbidden, web.CodeForbidden, "not a player in this match")
		case errors.Is(err, ErrNotSubmittable):
			web.Error(w, http.StatusConflict, web.CodeConflict, "match is not accepting submissions")
		case errors.Is(err, ErrAlreadySubmitted):
			web.Error(w, http.StatusConflict, web.CodeConflict, "already submitted")
		default:
			h.logger.Error("submit", "err", err)
			web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		}
		return
	}

	web.JSON(w, http.StatusAccepted, submitEnvelope{Match: submitMatch{
		ID: id, Status: res.Status, You: submitYou{Submitted: true, DrawingID: res.DrawingID},
	}})
}

// decodeSubmission reads {document} (8 MB cap), validates it at the write edge,
// and enforces the square game canvas (docs/GAME.md §2). Mirrors the drawings
// decode path plus the canvas-size check unique to a duel submission.
func (h *Handler) decodeSubmission(w http.ResponseWriter, r *http.Request) (document.Document, []byte, bool) {
	var req struct {
		Document json.RawMessage `json:"document"`
		// A client thumbnail may ride along but is advisory only and ignored here.
	}
	if err := web.DecodeJSONLax(w, r, &req, maxSubmitBodyBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			web.Error(w, http.StatusRequestEntityTooLarge, web.CodeDocumentTooLarge, "document exceeds the size limit")
		} else {
			web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "invalid request body")
		}
		return document.Document{}, nil, false
	}

	doc, err := document.ParseAndValidate(req.Document)
	if err != nil {
		msg := "invalid document"
		var ve *document.ValidationError
		if errors.As(err, &ve) {
			msg = ve.Msg
		}
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, msg)
		return document.Document{}, nil, false
	}

	// Both duelists share one honest space: a submission must be the square game
	// canvas (docs/GAME.md §2). Off-size is a validation error.
	if doc.Width != GameCanvasSize || doc.Height != GameCanvasSize {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed,
			fmt.Sprintf("submission must be %d×%d", GameCanvasSize, GameCanvasSize))
		return document.Document{}, nil, false
	}
	return doc, req.Document, true
}

// --- result ---

type resultEnvelope struct {
	Result any `json:"result"`
}

type resultPending struct {
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
}

type resultPlayerDTO struct {
	UserID       string   `json:"userId"`
	DisplayName  *string  `json:"displayName"`
	DrawingID    *string  `json:"drawingId"`
	Score        *float64 `json:"score"`
	RatingBefore *int32   `json:"ratingBefore"`
	RatingAfter  *int32   `json:"ratingAfter"`
	// JudgedImageURL points at the server-rendered authoritative raster in object
	// storage. Null until the object-storage seam + real render land (Phase 3 cont.).
	JudgedImageURL *string `json:"judgedImageUrl"`
}

type resultDone struct {
	Status       string            `json:"status"`
	Ready        bool              `json:"ready"`
	Prompt       promptDTO         `json:"prompt"`
	WinnerUserID *string           `json:"winnerUserId"`
	IsTie        bool              `json:"isTie"`
	Reason       *string           `json:"reason"`
	Players      []resultPlayerDTO `json:"players"`
}

// Result: GET /api/matches/{id}/result — the end-of-round result (auth: required;
// must be a player, else hidden 404). Both canvases are revealed once done.
func (h *Handler) Result(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context())
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
		return
	}

	view, err := h.svc.Result(r.Context(), uid, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
			return
		}
		h.logger.Error("result", "err", err)
		web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		return
	}
	web.JSON(w, http.StatusOK, resultEnvelope{Result: buildResultDTO(view)})
}

// buildResultDTO renders a ResultView: a compact {status, ready:false} while the
// round is in flight, the full verdict once done. Pure — table-tested directly.
func buildResultDTO(v ResultView) any {
	if !v.Ready {
		return resultPending{Status: v.Status, Ready: false}
	}
	players := make([]resultPlayerDTO, len(v.Players))
	for i, p := range v.Players {
		players[i] = resultPlayerDTO{
			UserID: p.UserID, DisplayName: p.DisplayName, DrawingID: p.DrawingID,
			Score: p.Score, RatingBefore: p.RatingBefore, RatingAfter: p.RatingAfter,
			JudgedImageURL: nil, // pending object storage + real render
		}
	}
	text := v.PromptText
	return resultDone{
		Status: v.Status, Ready: true,
		Prompt:       promptDTO{ID: v.PromptID, Text: &text},
		WinnerUserID: v.WinnerUserID,
		IsTie:        v.WinnerUserID == nil,
		Reason:       v.Reason,
		Players:      players,
	}
}
