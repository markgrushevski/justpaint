package drawings

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

const (
	maxDocumentBodyBytes = 8 << 20 // 8 MiB (docs/API.md §6)
	defaultPageLimit     = 20
	maxPageLimit         = 100
)

// Handler is the HTTP layer for drawings CRUD.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Routes registers the drawings routes; protect is the auth middleware (every
// route requires a session).
func (h *Handler) Routes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("POST /api/drawings", protect(http.HandlerFunc(h.Create)))
	mux.Handle("GET /api/drawings", protect(http.HandlerFunc(h.List)))
	mux.Handle("GET /api/drawings/{id}", protect(http.HandlerFunc(h.Get)))
	mux.Handle("PUT /api/drawings/{id}", protect(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /api/drawings/{id}", protect(http.HandlerFunc(h.Delete)))
}

// --- DTOs ---

type documentRequest struct {
	Document json.RawMessage `json:"document"`
	// A client `thumbnail` may ride along but is advisory only and ignored here.
}

type drawingMeta struct {
	ID           string    `json:"id"`
	OwnerID      string    `json:"ownerId"`
	MatchID      *string   `json:"matchId"`
	DocVersion   int32     `json:"docVersion"`
	Width        int32     `json:"width"`
	Height       int32     `json:"height"`
	ThumbnailURL *string   `json:"thumbnailUrl"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type drawingFull struct {
	drawingMeta
	Document json.RawMessage `json:"document"`
}

type metaEnvelope struct {
	Drawing drawingMeta `json:"drawing"`
}

type fullEnvelope struct {
	Drawing drawingFull `json:"drawing"`
}

type listEnvelope struct {
	Drawings   []drawingMeta `json:"drawings"`
	NextCursor *string       `json:"nextCursor"`
	Limit      int           `json:"limit"`
}

// --- handlers ---

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context()) // RequireAuth guarantees presence
	doc, raw, ok := h.decodeAndValidate(w, r)
	if !ok {
		return
	}
	d, err := h.svc.Create(r.Context(), uid, doc, raw)
	if err != nil {
		h.internal(w, "create drawing", err)
		return
	}
	web.JSON(w, http.StatusCreated, metaEnvelope{Drawing: toMeta(d)})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context())
	d, err := h.svc.Get(r.Context(), uid, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
			return
		}
		h.internal(w, "get drawing", err)
		return
	}
	web.JSON(w, http.StatusOK, fullEnvelope{Drawing: toFull(d)})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context())
	doc, raw, ok := h.decodeAndValidate(w, r)
	if !ok {
		return
	}
	d, err := h.svc.Update(r.Context(), uid, r.PathValue("id"), doc, raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
			return
		}
		h.internal(w, "update drawing", err)
		return
	}
	web.JSON(w, http.StatusOK, metaEnvelope{Drawing: toMeta(d)})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context())
	deleted, err := h.svc.Delete(r.Context(), uid, r.PathValue("id"))
	if err != nil {
		h.internal(w, "delete drawing", err)
		return
	}
	if !deleted {
		web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context())
	q := r.URL.Query()

	limit := parseLimit(q.Get("limit"))
	kind := q.Get("kind")
	if kind == "" {
		kind = "all"
	}
	if kind != "all" && kind != "free" && kind != "duel" {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "kind must be all, free, or duel")
		return
	}
	cur, err := decodeCursor(q.Get("cursor"))
	if err != nil {
		web.Error(w, http.StatusBadRequest, web.CodeValidationFailed, "invalid cursor")
		return
	}

	rows, err := h.svc.List(r.Context(), uid, kind, cur, int32(limit))
	if err != nil {
		h.internal(w, "list drawings", err)
		return
	}

	items := make([]drawingMeta, len(rows))
	for i, row := range rows {
		items[i] = rowToMeta(row)
	}
	var next *string
	if len(rows) == limit { // a full page ⇒ there may be more
		last := rows[len(rows)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		next = &c
	}
	web.JSON(w, http.StatusOK, listEnvelope{Drawings: items, NextCursor: next, Limit: limit})
}

// --- shared helpers ---

// decodeAndValidate reads the {document} body (8 MB cap), validates the vector
// document at the write edge, and reports the right HTTP error on failure.
func (h *Handler) decodeAndValidate(w http.ResponseWriter, r *http.Request) (document.Document, []byte, bool) {
	var req documentRequest
	if err := web.DecodeJSONLax(w, r, &req, maxDocumentBodyBytes); err != nil {
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
	return doc, req.Document, true
}

func (h *Handler) internal(w http.ResponseWriter, what string, err error) {
	h.logger.Error(what, "err", err)
	web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
}

func parseLimit(s string) int {
	if s == "" {
		return defaultPageLimit
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return defaultPageLimit
	}
	if n > maxPageLimit {
		return maxPageLimit
	}
	return n
}

// encodeCursor / decodeCursor make the keyset position opaque to clients
// (base64 of "<rfc3339nano>|<id>").
func encodeCursor(t time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeCursor(s string) (*cursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	at, id, found := strings.Cut(string(b), "|")
	if !found || id == "" {
		return nil, errors.New("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, err
	}
	return &cursor{createdAt: t, id: id}, nil
}

func toMeta(d db.Drawing) drawingMeta {
	return drawingMeta{
		ID: d.ID, OwnerID: d.OwnerID, MatchID: d.MatchID,
		DocVersion: d.DocVersion, Width: d.Width, Height: d.Height,
		ThumbnailURL: d.ThumbnailUrl, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func rowToMeta(d db.ListDrawingsRow) drawingMeta {
	return drawingMeta{
		ID: d.ID, OwnerID: d.OwnerID, MatchID: d.MatchID,
		DocVersion: d.DocVersion, Width: d.Width, Height: d.Height,
		ThumbnailURL: d.ThumbnailUrl, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func toFull(d db.Drawing) drawingFull {
	return drawingFull{drawingMeta: toMeta(d), Document: d.Document}
}
