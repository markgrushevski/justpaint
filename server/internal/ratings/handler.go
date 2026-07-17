package ratings

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

const (
	defaultLeaderboardLimit = 20
	maxLeaderboardLimit     = 100
)

// Handler is the HTTP layer for the leaderboard (docs/API.md §11).
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler builds the leaderboard HTTP handler over the read-only Service.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Routes registers the leaderboard route; protect is the auth middleware — the read
// is gated exactly like every other route (docs/API.md §11: auth required).
func (h *Handler) Routes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/leaderboard", protect(http.HandlerFunc(h.List)))
}

// --- DTOs ---

// leaderboardEntryDTO is one ranked row. displayName is nullable (no omitempty — a
// player without a display name serializes as null, not absent). rank is the
// 1-based row-number over the rating-ordered result (docs/API.md §11).
type leaderboardEntryDTO struct {
	Rank        int     `json:"rank"`
	UserID      string  `json:"userId"`
	DisplayName *string `json:"displayName"`
	Rating      int     `json:"rating"`
	GamesPlayed int     `json:"gamesPlayed"`
	Wins        int     `json:"wins"`
	Losses      int     `json:"losses"`
}

// leaderboardResponse wraps the ranked entries and echoes the clamped limit.
type leaderboardResponse struct {
	Leaderboard []leaderboardEntryDTO `json:"leaderboard"`
	Limit       int                   `json:"limit"`
}

// List: GET /api/leaderboard?limit=20 — the top-rated players (auth: required).
// `limit` is clamped (default 20, max 100), never 400 — the only client error is the
// 401 from RequireAuth. This deliberately deviates from the §7 keyset-cursor
// convention: rank is an absolute position a keyset can't carry, so the leaderboard
// is top-N by `limit` only (docs/API.md §11).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"))

	rows, err := h.svc.Top(r.Context(), int32(limit))
	if err != nil {
		h.internal(w, "leaderboard list", err)
		return
	}

	web.JSON(w, http.StatusOK, buildLeaderboard(rows, limit))
}

// buildLeaderboard assigns the row-number rank (i+1) over the already-ordered rows
// and maps each to its DTO. Pure — no I/O — so it is unit-tested directly. The
// returned slice is always non-nil (empty leaderboard serializes as [], not null).
func buildLeaderboard(rows []db.ListTopRatingsRow, limit int) leaderboardResponse {
	entries := make([]leaderboardEntryDTO, len(rows))
	for i, row := range rows {
		entries[i] = leaderboardEntryDTO{
			Rank:        i + 1,
			UserID:      row.ID,
			DisplayName: row.DisplayName,
			Rating:      int(row.Rating),
			GamesPlayed: int(row.GamesPlayed),
			Wins:        int(row.Wins),
			Losses:      int(row.Losses),
		}
	}
	return leaderboardResponse{Leaderboard: entries, Limit: limit}
}

// parseLimit clamps the ?limit query param (default 20, max 100). It never errors:
// a blank, non-numeric, or out-of-range value folds to a valid limit rather than a
// 400, so the endpoint has no client-error path beyond auth (copied semantics from
// drawings.parseLimit).
func parseLimit(s string) int {
	if s == "" {
		return defaultLeaderboardLimit
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return defaultLeaderboardLimit
	}
	if n > maxLeaderboardLimit {
		return maxLeaderboardLimit
	}
	return n
}

func (h *Handler) internal(w http.ResponseWriter, what string, err error) {
	h.logger.Error(what, "err", err)
	web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
}
