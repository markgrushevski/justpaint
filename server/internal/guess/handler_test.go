package guess

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/judge"
)

// testSecret signs the test session cookies; the same secret builds the auth
// handler so RequireAuth verifies them.
const testSecret = "guess-test-secret-please-ignore-00000000"

// sessionCookieName is pinned by docs/API.md §2 (the auth package's own const is
// unexported); the tests hardcode the wire name.
const sessionCookieName = "jp_session"

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

// muxFor wires the whole guess surface over a given service. No database is
// reachable from here because this module has none to reach.
func muxFor(t *testing.T, svc *Service) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	NewHandler(svc, slog.New(slog.DiscardHandler)).Routes(mux, authMiddleware(t))
	return mux
}

// unconfiguredMux is the whole surface with NO guesser — the JUDGE_MODE=http
// shape. Every request either fails at the edge (the validation cases) or gets
// the honest refusal.
func unconfiguredMux(t *testing.T) *http.ServeMux {
	t.Helper()
	return muxFor(t, NewService(&stubRenderer{}, nil, nil, nil, slog.New(slog.DiscardHandler)))
}

// workingMux is the fully wired shape: a renderer that returns a real raster and
// the in-process fake guesser, which is enough to prove the route end to end.
func workingMux(t *testing.T) *http.ServeMux {
	t.Helper()
	return muxFor(t, NewService(&stubRenderer{png: tinyPNG(t)}, judge.NewFakeGuesser(), nil, nil, slog.New(slog.DiscardHandler)))
}

func post(t *testing.T, mux *http.ServeMux, body string, authed bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/guess", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authed {
		req.AddCookie(mintCookie(t, svcUserID))
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

// The route requires a session: a call is billed to a player's allowance, so
// there is no anonymous path to it. RequireAuth owns this; asserted here so a
// future Routes() edit that drops the wrapper is caught.
func TestGuess_RequiresAuth(t *testing.T) {
	rec := post(t, unconfiguredMux(t), `{"document":`+docOfSize(800, 600)+`}`, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if code := errorCode(t, rec); code != "unauthorized" {
		t.Errorf("code = %q, want unauthorized", code)
	}
}

// The request edge, in one table. Every case here is refused before the service
// is reached.
func TestGuess_RejectsBadRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"not JSON at all", `{nope`},
		{"a document that is not a v1 document", `{"document":{"hello":"world"}}`},
		{"a document with no layers", `{"document":{"version":1,"width":800,"height":600,"background":null,"layers":[]}}`},
		{"a missing document", `{}`},
		{"a canvas past the format's own limit", `{"document":` + docOfSize(document.MaxCanvasDimension+1, 600) + `}`},
	}
	mux := unconfiguredMux(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(t, mux, tt.body, true)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			if code := errorCode(t, rec); code != "validation_failed" {
				t.Errorf("code = %q, want validation_failed", code)
			}
		})
	}
}

// The heart of this endpoint's validation choice: a free-draw canvas is ANY size
// the format allows. The duel's square 1080² rule (game.ValidateSubmission, which
// internal/practice reuses) belongs to the duel, where two drawings are compared
// with each other — importing it here would 400 exactly the drawings this feature
// exists to look at.
func TestGuess_AcceptsAnyFreeDrawCanvas(t *testing.T) {
	mux := workingMux(t)
	sizes := [][2]int{
		{800, 600},   // the common landscape scratch canvas
		{1080, 1080}, // the duel's size, which is allowed but not required
		{1, 1},       // the format's floor
		{4000, 3000}, // a big one, well past anything a duel would take
		{document.MaxCanvasDimension, document.MaxCanvasDimension},
	}
	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			rec := post(t, mux, `{"document":`+docOfSize(size[0], size[1])+`}`, true)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// The 8 MB document cap is the drawings/practice cap (docs/API.md §6) and answers
// with its own code, not a generic 400.
func TestGuess_RejectsOversizeBody(t *testing.T) {
	body := `{"document":{"version":1,"pad":"` + strings.Repeat("x", maxGuessBodyBytes) + `"}}`
	rec := post(t, unconfiguredMux(t), body, true)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 (body %s)", rec.Code, rec.Body.String())
	}
	if code := errorCode(t, rec); code != "document_too_large" {
		t.Errorf("code = %q, want document_too_large", code)
	}
}

// The answer's wire shape, which the frontend is written against.
func TestGuess_ReturnsTheFrozenEnvelope(t *testing.T) {
	rec := post(t, workingMux(t), `{"document":`+docOfSize(800, 600)+`}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	var env struct {
		Guess struct {
			Label        string   `json:"label"`
			Confidence   float64  `json:"confidence"`
			Alternatives []string `json:"alternatives"`
		} `json:"guess"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not the guess envelope (%s): %v", rec.Body.String(), err)
	}
	if env.Guess.Label == "" {
		t.Error("the envelope carried no label")
	}
	if env.Guess.Confidence < 0 || env.Guess.Confidence > 1 {
		t.Errorf("confidence = %v, outside [0,1]", env.Guess.Confidence)
	}
	// An empty list of runner-ups must be [] on the wire, never null: the client
	// renders 0-2 of them and should not have to know two spellings of "none".
	if !strings.Contains(rec.Body.String(), `"alternatives":[]`) {
		t.Errorf("empty alternatives must serialize as [], got %s", rec.Body.String())
	}
}

// JUDGE_MODE=http: the collaborator's service compares two drawings against a
// prompt and has nothing that names one drawing, so the guess is unconfigured. It
// must refuse — loudly — rather than quietly fall back to the ink-coverage fake,
// whose label a player has no way to tell from a real one.
func TestGuess_UnconfiguredRefusesHonestly(t *testing.T) {
	rec := post(t, unconfiguredMux(t), `{"document":`+docOfSize(800, 600)+`}`, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %s)", rec.Code, rec.Body.String())
	}
	if code := errorCode(t, rec); code != "internal" {
		t.Errorf("code = %q, want internal", code)
	}
	// A label must never appear in a refusal: the failure mode this guards against
	// is an invented answer that reads exactly like a real one.
	if strings.Contains(rec.Body.String(), `"label"`) {
		t.Errorf("an unconfigured guess returned something answer-shaped: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not available") {
		t.Errorf("the message should name the cause, got %s", rec.Body.String())
	}
}

// Both halves of the daily ceiling are 429 rate_limited, and the copy comes from
// aibudget — one decision about what a refusal discloses, not one per feature.
func TestGuess_BudgetRefusalsAre429(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantSaid string
	}{
		{
			name: "the player has had their turn",
			err:  &aibudget.KindSpentError{Kind: aibudget.KindGuess, Cap: 2},
			// The per-kind copy names the number, because it is the player's own and
			// small enough that they could have counted it themselves.
			wantSaid: "all 2 of your AI guesses",
		},
		{
			name:     "the service has had its turn",
			err:      aibudget.ErrGlobalSpent,
			wantSaid: "resumes tomorrow",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refuse := func(context.Context, string) error { return tt.err }
			renderer := &stubRenderer{png: tinyPNG(t)}
			mux := muxFor(t, NewService(renderer, judge.NewFakeGuesser(), refuse, nil, slog.New(slog.DiscardHandler)))

			rec := post(t, mux, `{"document":`+docOfSize(800, 600)+`}`, true)
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("status = %d, want 429 (body %s)", rec.Code, rec.Body.String())
			}
			if code := errorCode(t, rec); code != "rate_limited" {
				t.Errorf("code = %q, want rate_limited", code)
			}
			if !strings.Contains(rec.Body.String(), tt.wantSaid) {
				t.Errorf("body %s should mention %q", rec.Body.String(), tt.wantSaid)
			}
			// Refused before the expensive work, which is the whole point of a ceiling.
			if renderer.calls != 0 {
				t.Errorf("rendered %d times for a refused call", renderer.calls)
			}
		})
	}
}

// TestGuess_ProviderQuotaIs429 covers the third way to run out: not our own
// ceiling, but the provider's, learned from a 429 on the wire.
//
// It is the same news to the player — "not today" — so it reads the same. As a
// 500 it read as a fault and the UI offered a retry that could not possibly
// succeed until the provider's own window rolled. internal/game already
// special-cases this error in its judging pass for the same reason; the two
// inline endpoints were the ones still answering 500.
func TestGuess_ProviderQuotaIs429(t *testing.T) {
	renderer := &stubRenderer{png: tinyPNG(t)}
	guesser := &stubGuesser{err: fmt.Errorf("judge: %w (429): daily limit", judge.ErrQuotaExhausted)}
	mux := muxFor(t, NewService(renderer, guesser, nil, nil, slog.New(slog.DiscardHandler)))

	rec := post(t, mux, `{"document":`+docOfSize(800, 600)+`}`, true)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 (body %s)", rec.Code, rec.Body.String())
	}
	if code := errorCode(t, rec); code != "rate_limited" {
		t.Errorf("code = %q, want rate_limited", code)
	}
	// The same sentence the global half of our own ceiling writes — one decision
	// about what a refusal discloses, made once in aibudget.
	if !strings.Contains(rec.Body.String(), "resumes tomorrow") {
		t.Errorf("body %s should read like the budget's own global refusal", rec.Body.String())
	}
}
