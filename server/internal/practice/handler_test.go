package practice

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/game"
)

// testSecret signs the test session cookies; the same secret builds the auth
// handler so RequireAuth verifies them.
const testSecret = "practice-test-secret-please-ignore-00000"

// sessionCookieName is pinned by docs/API.md §2 (the auth package's own const is
// unexported); the tests hardcode the wire name.
const sessionCookieName = "jp_session"

const testUserID = "11111111-1111-4111-8111-111111111111"
const testPromptID = "22222222-2222-4222-8222-222222222222"

// docOfSize builds a minimal valid v1 document at the given canvas size.
func docOfSize(w, h int) string {
	return fmt.Sprintf(
		`{"version":1,"width":%d,"height":%d,"background":null,"layers":[{"id":"l","name":"L","visible":true,"opacity":1,"strokes":[]}]}`,
		w, h)
}

// authMiddleware builds the real RequireAuth over a nil-DB auth service
// (RequireAuth only reads the JWT secret, never the DB), so these tests exercise
// the production auth chain — including the 401 path.
func authMiddleware(t *testing.T) func(http.Handler) http.Handler {
	t.Helper()
	svc, err := auth.NewService(nil, testSecret)
	if err != nil {
		t.Fatalf("auth.NewService: %v", err)
	}
	return auth.NewHandler(svc, false, slog.New(slog.DiscardHandler)).RequireAuth
}

func mintCookie(t *testing.T, userID string) *http.Cookie {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: signed}
}

// unconfiguredMux is the whole practice surface with NO critic — the
// JUDGE_MODE=http shape. Every test below either fails before the service is
// reached (the validation cases) or asserts the honest refusal, so a nil
// *db.Queries here is never touched: that is the point of validating at the edge.
func unconfiguredMux(t *testing.T) *http.ServeMux {
	t.Helper()
	svc := NewService(nil, nil, nil, nil, nil, slog.New(slog.DiscardHandler))
	mux := http.NewServeMux()
	NewHandler(svc, slog.New(slog.DiscardHandler)).Routes(mux, authMiddleware(t))
	return mux
}

func post(t *testing.T, mux *http.ServeMux, body string, authed bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/practice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authed {
		req.AddCookie(mintCookie(t, testUserID))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// errorCode pulls the code out of the standard envelope (docs/API.md §3).
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not the error envelope (%s): %v", rec.Body.String(), err)
	}
	return env.Error.Code
}

// Both routes require a session. RequireAuth owns this; asserted here so a future
// Routes() edit that drops the wrapper is caught.
func TestPractice_RequiresAuth(t *testing.T) {
	mux := unconfiguredMux(t)

	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{
		{"prompt", http.MethodGet, "/api/practice/prompt"},
		{"run", http.MethodPost, "/api/practice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`)))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
			if code := errorCode(t, rec); code != "unauthorized" {
				t.Errorf("code = %q, want unauthorized", code)
			}
		})
	}
}

// The request edge, in one table. Every case here is refused before the service is
// reached, which is why a service with no database can serve them.
func TestPracticeRun_RejectsBadRequests(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantErr  string
		wantSaid string
	}{
		{
			name:     "not JSON at all",
			body:     `{nope`,
			wantCode: http.StatusBadRequest,
			wantErr:  "validation_failed",
		},
		{
			name:     "a document that is not a v1 document",
			body:     `{"promptId":"` + testPromptID + `","document":{"hello":"world"}}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "validation_failed",
		},
		{
			name:     "a document with no layers",
			body:     `{"promptId":"` + testPromptID + `","document":{"version":1,"width":1080,"height":1080,"background":null,"layers":[]}}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "validation_failed",
		},
		{
			// The canvas rule is the duel's, reused verbatim: a practice drawing is
			// scored the same way, so it must be the same size.
			name:     "a document off the game canvas",
			body:     `{"promptId":"` + testPromptID + `","document":` + docOfSize(800, 600) + `}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "validation_failed",
			wantSaid: fmt.Sprintf("%d×%d", game.GameCanvasSize, game.GameCanvasSize),
		},
		{
			name:     "a missing document",
			body:     `{"promptId":"` + testPromptID + `"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "validation_failed",
		},
		{
			// A non-UUID id can never name a row, so it is hidden as 404 exactly like a
			// foreign one rather than reaching the ::uuid cast as a 500.
			name:     "a promptId that is not a uuid",
			body:     `{"promptId":"not-a-uuid","document":` + docOfSize(game.GameCanvasSize, game.GameCanvasSize) + `}`,
			wantCode: http.StatusNotFound,
			wantErr:  "not_found",
		},
		{
			name:     "a missing promptId",
			body:     `{"document":` + docOfSize(game.GameCanvasSize, game.GameCanvasSize) + `}`,
			wantCode: http.StatusNotFound,
			wantErr:  "not_found",
		},
	}
	mux := unconfiguredMux(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(t, mux, tt.body, true)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if code := errorCode(t, rec); code != tt.wantErr {
				t.Errorf("code = %q, want %q", code, tt.wantErr)
			}
			if tt.wantSaid != "" && !strings.Contains(rec.Body.String(), tt.wantSaid) {
				t.Errorf("body %s should mention %q", rec.Body.String(), tt.wantSaid)
			}
		})
	}
}

// The 8 MB document cap is the drawings/submit cap (docs/API.md §6) and answers
// with its own code, not a generic 400.
func TestPracticeRun_RejectsOversizeBody(t *testing.T) {
	body := `{"promptId":"` + testPromptID + `","document":{"version":1,"pad":"` +
		strings.Repeat("x", maxRunBodyBytes) + `"}}`
	rec := post(t, unconfiguredMux(t), body, true)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 (body %s)", rec.Code, rec.Body.String())
	}
	if code := errorCode(t, rec); code != "document_too_large" {
		t.Errorf("code = %q, want document_too_large", code)
	}
}

// JUDGE_MODE=http: the collaborator's service scores two drawings against each
// other and has no critique endpoint, so practice is unconfigured. It must refuse
// — loudly and on BOTH routes — rather than quietly fall back to the ink-coverage
// fake, whose number a player has no way to tell from a real one.
func TestPractice_UnconfiguredRefusesHonestly(t *testing.T) {
	mux := unconfiguredMux(t)

	t.Run("run", func(t *testing.T) {
		body := `{"promptId":"` + testPromptID + `","document":` + docOfSize(game.GameCanvasSize, game.GameCanvasSize) + `}`
		rec := post(t, mux, body, true)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500 (body %s)", rec.Code, rec.Body.String())
		}
		if code := errorCode(t, rec); code != "internal" {
			t.Errorf("code = %q, want internal", code)
		}
		// A score must never appear in a refusal: the failure mode this guards
		// against is an invented number that reads exactly like a real one.
		if strings.Contains(rec.Body.String(), `"score"`) {
			t.Errorf("an unconfigured practice returned something score-shaped: %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "not available") {
			t.Errorf("the message should name the cause, got %s", rec.Body.String())
		}
	})

	t.Run("prompt", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/practice/prompt", nil)
		req.AddCookie(mintCookie(t, testUserID))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		// Handing out a prompt we cannot score would walk the player through a whole
		// drawing before admitting it.
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500 (body %s)", rec.Code, rec.Body.String())
		}
	})
}
