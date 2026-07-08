// Package drawings is the CRUD for vector drawings: validate the document at the
// write edge, store it as jsonb, and scope every operation to its owner.
// See docs/API.md §7, docs/DOCUMENT-FORMAT.md §7.
package drawings

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
)

// ErrDuelLocked marks a write/delete against a drawing that is a submitted duel
// entry (match_id set): immutable via CRUD (docs/API.md §7 — a submitted duel
// drawing is locked). The handler maps it to 409 conflict, distinct from the 404
// a genuinely absent/foreign row gets.
var ErrDuelLocked = errors.New("drawings: duel submission is immutable")

// Service holds the drawings business logic over the generated queries.
type Service struct {
	q *db.Queries
}

func NewService(q *db.Queries) *Service { return &Service{q: q} }

// cursor is the decoded keyset-pagination position (newest-first on created_at, id).
type cursor struct {
	createdAt time.Time
	id        string
}

// Create stores a new free-draw drawing (match_id null). The server derives
// doc_version/width/height from the validated document; raw is the canonical
// jsonb payload (unknown fields preserved). name is normalized metadata from
// the handler; nil ⇒ the SQL default 'new art' (queries/drawings.sql).
func (s *Service) Create(ctx context.Context, ownerID string, name *string, doc document.Document, raw []byte) (db.Drawing, error) {
	// int32 narrowing is safe: ParseAndValidate already bounds version==1 and
	// width/height to [1,8192] before the service is ever reached.
	return s.q.CreateDrawing(ctx, db.CreateDrawingParams{
		OwnerID:    ownerID,
		MatchID:    nil, // free /draw save; duel submissions go through the game route
		Name:       name,
		DocVersion: int32(doc.Version),
		Width:      int32(doc.Width),
		Height:     int32(doc.Height),
		Document:   raw,
	})
}

func (s *Service) Get(ctx context.Context, ownerID, id string) (db.Drawing, error) {
	return s.q.GetDrawing(ctx, db.GetDrawingParams{ID: id, OwnerID: ownerID})
}

// Update replaces an owned free drawing's document. name nil (absent/blank in
// the request) KEEPS the stored name — the query coalesces, so no read-modify-
// write round trip is needed; non-nil REPLACES it.
func (s *Service) Update(ctx context.Context, ownerID, id string, name *string, doc document.Document, raw []byte) (db.Drawing, error) {
	d, err := s.q.UpdateDrawing(ctx, db.UpdateDrawingParams{
		ID:         id,
		OwnerID:    ownerID,
		Name:       name,
		DocVersion: int32(doc.Version),
		Width:      int32(doc.Width),
		Height:     int32(doc.Height),
		Document:   raw,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if miss := s.classifyMiss(ctx, ownerID, id); miss != nil {
			return db.Drawing{}, miss // ErrDuelLocked → 409
		}
		return db.Drawing{}, pgx.ErrNoRows // genuinely absent/foreign → 404
	}
	return d, err
}

// Delete removes an owned FREE drawing and reports whether a row was actually
// deleted (false ⇒ not found / not owned ⇒ the handler answers 404). A submitted
// duel drawing is immutable ⇒ ErrDuelLocked (→ 409).
func (s *Service) Delete(ctx context.Context, ownerID, id string) (bool, error) {
	rows, err := s.q.DeleteDrawing(ctx, db.DeleteDrawingParams{ID: id, OwnerID: ownerID})
	if err != nil {
		return false, err
	}
	if rows == 0 {
		if miss := s.classifyMiss(ctx, ownerID, id); miss != nil {
			return false, miss
		}
		return false, nil // genuinely absent / foreign ⇒ 404
	}
	return true, nil
}

// classifyMiss explains why an owner-scoped Update/Delete touched no row: the
// write queries carry `match_id is null`, so a duel submission (owned but
// match-linked) matches nothing. If the row still exists for this owner, it must
// be that locked duel entry ⇒ ErrDuelLocked; otherwise it is genuinely
// absent/foreign ⇒ nil (the handler answers 404). match_id is fixed at creation,
// so this classification is race-free.
func (s *Service) classifyMiss(ctx context.Context, ownerID, id string) error {
	if _, err := s.q.GetDrawing(ctx, db.GetDrawingParams{ID: id, OwnerID: ownerID}); err == nil {
		return ErrDuelLocked
	}
	return nil
}

func (s *Service) List(ctx context.Context, ownerID, kind string, cur *cursor, limit int32) ([]db.ListDrawingsRow, error) {
	params := db.ListDrawingsParams{OwnerID: ownerID, Kind: kind, PageLimit: limit}
	if cur != nil {
		t := cur.createdAt
		id := cur.id
		params.CursorCreatedAt = &t
		params.CursorID = &id
	}
	return s.q.ListDrawings(ctx, params)
}
