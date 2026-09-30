package game

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
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
	mux.Handle("GET /api/matches/{id}/players/{userId}/drawing", protect(http.HandlerFunc(h.PlayerDrawing)))
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
	ID      string      `json:"id"`
	Mode    string      `json:"mode"`
	Status  string      `json:"status"`
	Prompt  promptDTO   `json:"prompt"`
	Canvas  canvasDTO   `json:"canvas"`
	Players []playerDTO `json:"players"`
	// DrawingDeadline is the absolute round deadline (RFC3339Nano, UTC), null while
	// `open`; ServerTime lets the client correct clock skew before counting down
	// (docs/API.md §8).
	DrawingDeadline *string   `json:"drawingDeadline"`
	ServerTime      string    `json:"serverTime"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type matchEnvelope struct {
	Match matchDTO `json:"match"`
}

// formatDeadline renders an optional deadline as RFC3339Nano (UTC), or nil while
// `open`, in the same format as serverTime so the client parses one shape.
func formatDeadline(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

// buildMatchDTO renders a MatchView for one viewer, applying the two visibility
// rules (docs/GAME.md §4.2, §5): prompt text hidden until `open` ends, and a
// player sees only their own drawingId until `done`. Pure — table-tested directly.
func buildMatchDTO(v MatchView, viewerID string, now time.Time) matchDTO {
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
		ID:              v.ID,
		Mode:            v.Mode,
		Status:          v.Status,
		Prompt:          prompt,
		Canvas:          canvasDTO{Width: document.ScoredCanvasSize, Height: document.ScoredCanvasSize},
		Players:         players,
		DrawingDeadline: formatDeadline(v.DrawingDeadline),
		ServerTime:      now.UTC().Format(time.RFC3339Nano),
		CreatedAt:       v.CreatedAt,
		UpdatedAt:       v.UpdatedAt,
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
	// v1 accepts only "async" (the default), validated but not threaded into
	// CreateOrJoin: creation relies on the matches.mode column default, and live
	// realtime added no 'live' mode (docs/GAME.md §9) — there is nowhere else for
	// the value to go.
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
		// Both budget refusals are 429s but not the same news; the copy for both
		// lives in one place (aibudget.WriteRefusal). Checked before the switch,
		// not as a case predicate, because it writes the response as a side effect.
		if aibudget.WriteRefusal(w, err) {
			return
		}
		switch {
		case errors.Is(err, ErrNoPrompts):
			h.logger.Error("create match: no active prompts — run the seed migration (00002)")
			web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		default:
			h.logger.Error("create match", "err", err)
			web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		}
		return
	}
	web.JSON(w, http.StatusCreated, matchEnvelope{Match: buildMatchDTO(view, uid, time.Now())})
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
	web.JSON(w, http.StatusOK, matchEnvelope{Match: buildMatchDTO(view, uid, time.Now())})
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
	// Same deadline/clock pair as matchDTO, so the submit ack re-anchors the
	// client countdown without a follow-up GET (docs/API.md §8, submit).
	DrawingDeadline *string `json:"drawingDeadline"`
	ServerTime      string  `json:"serverTime"`
}

type submitEnvelope struct {
	Match submitMatch `json:"match"`
}

// Submit: POST /api/matches/{id}/submit — submit the caller's vector document for
// this match (auth: required; must be a player). Returns 202: the submission is
// recorded, the verdict is produced out-of-band (docs/API.md §8, submit).
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
		case errors.Is(err, ErrRoundExpired):
			// The match moved on (forfeit/abandon); the client treats a 409 here as
			// "go poll the result", not an error toast (docs/API.md §8, submit).
			web.Error(w, http.StatusConflict, web.CodeConflict, "round expired")
		default:
			h.logger.Error("submit", "err", err)
			web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		}
		return
	}

	web.JSON(w, http.StatusAccepted, submitEnvelope{Match: submitMatch{
		ID: id, Status: res.Status, You: submitYou{Submitted: true, DrawingID: res.DrawingID},
		DrawingDeadline: formatDeadline(res.Deadline), ServerTime: time.Now().UTC().Format(time.RFC3339Nano),
	}})
}

// decodeSubmission reads {document} (8 MB cap), validates it at the write edge,
// and enforces the square game canvas (docs/GAME.md §2).
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

	doc, err := document.ValidateScored(req.Document)
	if err != nil {
		msg := "invalid document"
		var ve *document.ValidationError
		if errors.As(err, &ve) {
			msg = ve.Msg
		}
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, msg)
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
	// JudgedImageURL would point at the authoritative raster in object storage,
	// dropped in favor of the participant-drawing route plus a client render
	// (docs/API.md §8, result; docs/DECISIONS.md 2026-07-11); stays null.
	JudgedImageURL *string `json:"judgedImageUrl"`
}

type resultDone struct {
	Status       string    `json:"status"`
	Ready        bool      `json:"ready"`
	Prompt       promptDTO `json:"prompt"`
	WinnerUserID *string   `json:"winnerUserId"`
	IsTie        bool      `json:"isTie"`
	Reason       *string   `json:"reason"`
	// Resolution is how the match was decided: 'judged' | 'forfeit' | 'aborted'. The
	// client branches on this, never on the free-text Reason (docs/GAME.md §4.1).
	Resolution string            `json:"resolution"`
	Players    []resultPlayerDTO `json:"players"`
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

// --- participant drawing (the result reveal) ---

// playerDrawingEnvelope carries the raw vector document inline (json.RawMessage,
// not a base64 []byte), so the client renders it with the editor renderer.
type playerDrawingEnvelope struct {
	Document json.RawMessage `json:"document"`
}

// PlayerDrawing: GET .../players/{userId}/drawing — a fellow participant's
// submitted document, revealed only once `done` (auth required; caller must be
// a co-player, else hidden 404). Authorization is match membership, not
// ownership: GET /api/drawings/{id} 404s a non-owner and can't serve the
// opponent's canvas (docs/API.md §8). No object storage.
func (h *Handler) PlayerDrawing(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context())
	matchID := r.PathValue("id")
	targetID := r.PathValue("userId")
	// A non-UUID id can never name a row; hide it as 404 (uniform with foreign ids)
	// rather than letting it reach the ::uuid cast as a 500.
	if _, err := uuid.Parse(matchID); err != nil {
		web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
		return
	}
	if _, err := uuid.Parse(targetID); err != nil {
		web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
		return
	}

	doc, err := h.svc.PlayerDrawing(r.Context(), uid, matchID, targetID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
			return
		}
		h.logger.Error("player drawing", "err", err)
		web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		return
	}
	web.JSON(w, http.StatusOK, playerDrawingEnvelope{Document: doc})
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
			JudgedImageURL: nil, // no object storage — see resultPlayerDTO
		}
	}
	text := v.PromptText
	// Default nil (legacy pre-migration `done` rows) to 'judged' so the field is
	// never empty on a completed match (docs/API.md §8, result).
	resolution := resolutionJudged
	if v.Resolution != nil {
		resolution = *v.Resolution
	}
	return resultDone{
		Status: v.Status, Ready: true,
		Prompt:       promptDTO{ID: v.PromptID, Text: &text},
		WinnerUserID: v.WinnerUserID,
		// A tie is a verdict with no winner; an aborted round has no winner either
		// but produced no verdict at all, so it must not read as a drawn duel —
		// resolution is the only thing that distinguishes them (docs/API.md §8, result).
		IsTie:      v.WinnerUserID == nil && resolution != resolutionAborted,
		Reason:     v.Reason,
		Resolution: resolution,
		Players:    players,
	}
}
