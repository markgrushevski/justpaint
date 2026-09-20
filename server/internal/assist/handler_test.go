package assist

import (
	"context"
	"encoding/json"
	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/document"
)

// testSecret signs the test session cookies; the same secret builds the auth
// handler so RequireAuth verifies them.
const testSecret = "assist-test-secret-please-ignore-0000000"

// sessionCookieName is pinned by docs/API.md §2 (the auth package's own const is
// unexported); the tests hardcode the wire name.
const sessionCookieName = "jp_session"

const validBody = `{"prompt":"draw a house","docSummary":{"canvas":{"width":1080,"height":1080},"layers":[]},"targetLayerId":null}`

// authMiddleware builds the real RequireAuth middleware over a nil-DB auth service
// (RequireAuth only reads the JWT secret, never the DB), so the assist tests
// exercise the exact production auth chain — including the 401 path.
func authMiddleware(t *testing.T) func(http.Handler) http.Handler {
	t.Helper()
	svc, err := auth.NewService(nil, testSecret)
	if err != nil {
		t.Fatalf("auth.NewService: %v", err)
	}
	return auth.NewHandler(svc, false, slog.New(slog.DiscardHandler)).RequireAuth
}

// mintCookie signs a jp_session cookie for userID (same claim shape parseToken
// verifies: HS256, subject, required exp).
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

// errAssist returns a fixed error from GenerateOps (drives the ErrInvalidBatch and
// internal-error paths).
type errAssist struct{ err error }

func (e errAssist) GenerateOps(context.Context, Request) (Result, error) { return Result{}, e.err }

// badOpsAssist returns a syntactically fine but semantically invalid batch (an
// add_stroke onto a layer that was never created) — the handler's defensive
// re-validation must reject it with 400.
type badOpsAssist struct{}

func (badOpsAssist) GenerateOps(context.Context, Request) (Result, error) {
	sw := 3.0
	return Result{Ops: []document.Op{
		&document.AddStrokeOp{Kind: document.OpAddStroke, LayerID: "ghost", Stroke: &document.RectStroke{
			StrokeBase:  document.StrokeBase{ID: "s1", Type: document.StrokeRect, Composite: document.CompositeSourceOver},
			X:           10,
			Y:           10,
			Width:       50,
			Height:      50,
			StrokeWidth: &sw,
		}},
	}}, nil
}

// serve runs one request through the assist handler behind RequireAuth.
func serve(t *testing.T, h *Handler, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler := authMiddleware(t)(http.HandlerFunc(h.GenerateOps))
	req := httptest.NewRequest(http.MethodPost, "/api/assist/ops", strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// errorCode extracts the code from the standard error envelope.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v (body: %s)", err, rec.Body)
	}
	return env.Error.Code
}

func TestGenerateOps(t *testing.T) {
	// A prompt one byte over the cap, wrapped in an otherwise-valid body (stays well
	// under the 64 KiB body cap, so it reaches the prompt guard, not the decoder).
	longPromptBody := `{"prompt":"` + strings.Repeat("a", maxPromptBytes+1) +
		`","docSummary":{"canvas":{"width":1080,"height":1080},"layers":[]},"targetLayerId":null}`

	tests := []struct {
		name       string
		impl       Assist
		withCookie bool
		body       string
		wantStatus int
		wantCode   string // "" for a 2xx
	}{
		{
			name:       "200 happy path — fake ops validate",
			impl:       NewFakeAssist(),
			withCookie: true,
			body:       validBody,
			wantStatus: http.StatusOK,
		},
		{
			name:       "401 without a session cookie",
			impl:       NewFakeAssist(),
			withCookie: false,
			body:       validBody,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthorized",
		},
		{
			name:       "400 on an unknown field (strict decode)",
			impl:       NewFakeAssist(),
			withCookie: true,
			body:       `{"prompt":"x","docSummary":{"canvas":{"width":1,"height":1},"layers":[]},"bogus":true}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
		},
		{
			name:       "400 on malformed JSON",
			impl:       NewFakeAssist(),
			withCookie: true,
			body:       `{not json`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
		},
		{
			name:       "400 on a whitespace-only prompt",
			impl:       NewFakeAssist(),
			withCookie: true,
			body:       `{"prompt":"   ","docSummary":{"canvas":{"width":1080,"height":1080},"layers":[]},"targetLayerId":null}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
		},
		{
			name:       "400 on an over-long prompt",
			impl:       NewFakeAssist(),
			withCookie: true,
			body:       longPromptBody,
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
		},
		{
			name:       "400 when the impl exhausts retries (ErrInvalidBatch)",
			impl:       errAssist{err: ErrInvalidBatch},
			withCookie: true,
			body:       validBody,
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
		},
		{
			name:       "400 when the impl returns invalid ops (defense re-validation)",
			impl:       badOpsAssist{},
			withCookie: true,
			body:       validBody,
			wantStatus: http.StatusBadRequest,
			wantCode:   "validation_failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(tc.impl, NewRateLimiter(DefaultBurst, time.Minute), nil, nil, slog.New(slog.DiscardHandler))
			var cookie *http.Cookie
			if tc.withCookie {
				cookie = mintCookie(t, "u1")
			}
			rec := serve(t, h, cookie, tc.body)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tc.wantStatus, rec.Body)
			}
			if tc.wantCode != "" {
				if got := errorCode(t, rec); got != tc.wantCode {
					t.Errorf("code = %q, want %q", got, tc.wantCode)
				}
			}
			if tc.wantStatus == http.StatusOK {
				// The 200 body must carry the validated batch + note.
				var resp struct {
					Ops  []json.RawMessage `json:"ops"`
					Note string            `json:"note"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode ops response: %v", err)
				}
				if len(resp.Ops) == 0 {
					t.Error("expected a non-empty ops batch in the 200 response")
				}
			}
		})
	}
}

// TestGenerateOps_RateLimited drives the per-user bucket: with burst=1 the second
// request is throttled with 429 rate_limited AND a Retry-After header (set before
// web.Error, per the ordering gotcha).
func TestGenerateOps_RateLimited(t *testing.T) {
	h := NewHandler(NewFakeAssist(), NewRateLimiter(1, time.Minute), nil, nil, slog.New(slog.DiscardHandler))
	cookie := mintCookie(t, "u1")

	if rec := serve(t, h, cookie, validBody); rec.Code != http.StatusOK {
		t.Fatalf("request 1 status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	rec := serve(t, h, cookie, validBody)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("request 2 status = %d, want 429; body: %s", rec.Code, rec.Body)
	}
	if got := errorCode(t, rec); got != "rate_limited" {
		t.Errorf("code = %q, want rate_limited", got)
	}
	if ra := rec.Header().Get("Retry-After"); ra == "" {
		t.Error("missing Retry-After header on 429")
	}
}

// TestGenerateOps_DailyBudget drives the OTHER ceiling on this endpoint — the
// durable daily one, as opposed to the in-process token bucket above. The two
// answer different questions (a rate versus a day's quota) and both land as 429
// rate_limited, which is the layering docs/API.md §3.1 already documents for
// POST /api/matches.
//
// The order matters and is asserted here: the budget is consulted AFTER the cheap
// request guards and BEFORE the call that costs money, and the call is RECORDED
// before it is made — a provider request that fails still spent the quota, and a
// failure the budget cannot see is what a broken impl drains it through.
func TestGenerateOps_DailyBudget(t *testing.T) {
	t.Run("a refusal is a 429 naming assist, and no call is made", func(t *testing.T) {
		called := false
		impl := callCountingAssist{onCall: func() { called = true }}
		refuse := func(context.Context, string) error {
			return &aibudget.KindSpentError{Kind: aibudget.KindAssist, Cap: 40}
		}
		h := NewHandler(impl, NewRateLimiter(DefaultBurst, time.Minute), refuse, nil, slog.New(slog.DiscardHandler))

		rec := serve(t, h, mintCookie(t, "u1"), validBody)
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d, want 429; body: %s", rec.Code, rec.Body)
		}
		if got := errorCode(t, rec); got != "rate_limited" {
			t.Errorf("code = %q, want rate_limited", got)
		}
		if !strings.Contains(rec.Body.String(), "AI drawing requests") {
			t.Errorf("the refusal should name assist, got: %s", rec.Body)
		}
		if called {
			t.Error("the impl was called despite the budget refusing — the whole point is to refuse BEFORE paying")
		}
	})

	t.Run("a served request is recorded before the call", func(t *testing.T) {
		var order []string
		impl := callCountingAssist{onCall: func() { order = append(order, "call") }}
		spend := func(context.Context, ...string) error {
			order = append(order, "spend")
			return nil
		}
		h := NewHandler(impl, NewRateLimiter(DefaultBurst, time.Minute),
			func(context.Context, string) error { return nil }, spend, slog.New(slog.DiscardHandler))

		if rec := serve(t, h, mintCookie(t, "u1"), validBody); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
		}
		if len(order) != 2 || order[0] != "spend" || order[1] != "call" {
			t.Errorf("order = %v, want [spend call]", order)
		}
	})
}

// callCountingAssist is FakeAssist with a hook, so a test can tell whether the
// expensive half ran at all.
type callCountingAssist struct{ onCall func() }

func (c callCountingAssist) GenerateOps(ctx context.Context, req Request) (Result, error) {
	c.onCall()
	return NewFakeAssist().GenerateOps(ctx, req)
}
