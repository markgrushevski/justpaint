// Package drawings is the CRUD for vector drawings: validate the document at the
// write edge, store it as jsonb, and scope every operation to its owner.
// See docs/API.md §7, docs/DOCUMENT-FORMAT.md §7.
package drawings

import (
	"context"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
)

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
// jsonb payload (unknown fields preserved).
func (s *Service) Create(ctx context.Context, ownerID string, doc document.Document, raw []byte) (db.Drawing, error) {
	// int32 narrowing is safe: ParseAndValidate already bounds version==1 and
	// width/height to [1,8192] before the service is ever reached.
	return s.q.CreateDrawing(ctx, db.CreateDrawingParams{
		OwnerID:    ownerID,
		MatchID:    nil, // free /draw save; duel submissions go through the game route
		DocVersion: int32(doc.Version),
		Width:      int32(doc.Width),
		Height:     int32(doc.Height),
		Document:   raw,
	})
}

func (s *Service) Get(ctx context.Context, ownerID, id string) (db.Drawing, error) {
	return s.q.GetDrawing(ctx, db.GetDrawingParams{ID: id, OwnerID: ownerID})
}

func (s *Service) Update(ctx context.Context, ownerID, id string, doc document.Document, raw []byte) (db.Drawing, error) {
	return s.q.UpdateDrawing(ctx, db.UpdateDrawingParams{
		ID:         id,
		OwnerID:    ownerID,
		DocVersion: int32(doc.Version),
		Width:      int32(doc.Width),
		Height:     int32(doc.Height),
		Document:   raw,
	})
}

// Delete removes an owned drawing and reports whether a row was actually deleted
// (false ⇒ not found / not owned ⇒ the handler answers 404).
func (s *Service) Delete(ctx context.Context, ownerID, id string) (bool, error) {
	rows, err := s.q.DeleteDrawing(ctx, db.DeleteDrawingParams{ID: id, OwnerID: ownerID})
	return rows > 0, err
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
