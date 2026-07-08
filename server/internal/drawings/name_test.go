package drawings

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

// TestNormalizeName pins the drawing-name rules (docs/API.md §7): trim, blank ⇒
// nil (SQL defaults it on create / keeps it on update), cap at 64 RUNES —
// multibyte names count characters, not bytes, mirroring the document
// validator's layer-name cap.
func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    *string // nil = absent → default/keep at the SQL layer
		wantErr bool
	}{
		{"absent/empty → nil (default applies)", "", nil, false},
		{"whitespace only → nil", " \t\n  ", nil, false},
		{"plain name passes through", "sunset study", strptr("sunset study"), false},
		{"surrounding spaces trimmed", "  sunset study \t", strptr("sunset study"), false},
		{"exactly 64 ASCII runes ok", strings.Repeat("a", 64), strptr(strings.Repeat("a", 64)), false},
		{"65 ASCII runes rejected", strings.Repeat("a", 65), nil, true},
		// 64 runes but 4 bytes each — a byte-counting cap would wrongly reject this.
		{"exactly 64 multibyte runes ok", strings.Repeat("\U0001F3A8", 64), strptr(strings.Repeat("\U0001F3A8", 64)), false},
		{"65 multibyte runes rejected", strings.Repeat("\U0001F3A8", 65), nil, true},
		{"cap measured after trimming", "  " + strings.Repeat("é", 64) + "  ", strptr(strings.Repeat("é", 64)), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeName(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeName(%q) = %v, want error", tc.in, got)
				}
				if !strings.Contains(err.Error(), "64") {
					t.Errorf("error %q does not mention the 64 cap", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeName(%q) unexpected error: %v", tc.in, err)
			}
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("normalizeName(%q) = %q, want nil", tc.in, *got)
			case tc.want != nil && got == nil:
				t.Errorf("normalizeName(%q) = nil, want %q", tc.in, *tc.want)
			case tc.want != nil && got != nil && *got != *tc.want:
				t.Errorf("normalizeName(%q) = %q, want %q", tc.in, *got, *tc.want)
			}
		})
	}
}

// TestCreate_NameTooLong drives the full HTTP write edge: an over-cap name must
// be rejected with the standard 400 validation_failed envelope, before any
// persistence is attempted (the service is backed by nil queries — reaching the
// DB would panic).
func TestCreate_NameTooLong(t *testing.T) {
	h := NewHandler(NewService(nil), slog.New(slog.DiscardHandler))

	body := `{"name":"` + strings.Repeat("x", 65) + `","document":` + minimalDocJSON + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/drawings", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != "validation_failed" {
		t.Errorf("code = %q, want validation_failed", env.Error.Code)
	}
	if !strings.Contains(env.Error.Message, "64") {
		t.Errorf("message %q does not mention the 64 cap", env.Error.Message)
	}
}

// TestMetaCarriesName pins that both meta mappers surface the name column in
// the DTO (docs/API.md §7 response shapes).
func TestMetaCarriesName(t *testing.T) {
	now := time.Now()
	if got := toMeta(db.Drawing{ID: "d1", Name: "sunset study", CreatedAt: now, UpdatedAt: now}).Name; got != "sunset study" {
		t.Errorf("toMeta name = %q, want %q", got, "sunset study")
	}
	if got := rowToMeta(db.ListDrawingsRow{ID: "d1", Name: "sunset study", CreatedAt: now, UpdatedAt: now}).Name; got != "sunset study" {
		t.Errorf("rowToMeta name = %q, want %q", got, "sunset study")
	}
}

func strptr(s string) *string { return &s }
