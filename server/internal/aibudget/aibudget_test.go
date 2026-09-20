package aibudget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseKind(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   Kind
		wantOK bool
	}{
		{"duel", "duel", KindDuel, true},
		{"practice", "practice", KindPractice, true},
		{"guess", "guess", KindGuess, true},
		{"assist", "assist", KindAssist, true},
		// The boot-time job: a typo must not quietly configure an allowance nothing
		// reads, leaving the feature it was meant to bound running unbudgeted.
		{"typo", "duels", "", false},
		{"empty", "", "", false},
		{"unknown feature", "inpaint", "", false},
		// Case is the caller's to normalize (config.Load lowercases every other mode
		// env before it compares), so this package stays strict about it.
		{"wrong case", "Duel", "", false},
		{"padded", " duel ", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseKind(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("ParseKind(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("ParseKind(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestAllKindsIsComplete guards the three places a Kind must appear together. A
// kind that AllKinds omits is one an operator cannot configure and a boot error
// cannot name; one that DefaultPerUser omits ships with a zero allowance, which
// would refuse every call the day a real provider is wired.
func TestAllKindsIsComplete(t *testing.T) {
	kinds := AllKinds()
	if len(kinds) == 0 {
		t.Fatal("AllKinds is empty")
	}
	seen := make(map[Kind]bool, len(kinds))
	for _, k := range kinds {
		if seen[k] {
			t.Errorf("AllKinds lists %q twice", k)
		}
		seen[k] = true

		if got, ok := ParseKind(string(k)); !ok || got != k {
			t.Errorf("ParseKind(%q) = %q, %v — AllKinds and ParseKind disagree", k, got, ok)
		}
		if n, ok := DefaultPerUser[k]; !ok || n < 1 {
			t.Errorf("DefaultPerUser[%q] = %d, %v — want a default of at least 1", k, n, ok)
		}
		// A kind with no noun of its own refuses in the generic sentence, which is a
		// worse message than the one it could have had. One template means the noun is
		// the ONLY per-kind word left, so it is the only one that can go missing.
		if noun := k.Noun(); noun == fallbackNoun {
			t.Errorf("Kind(%q).Noun() falls back to %q — every shipped kind should name itself", k, fallbackNoun)
		}
	}
	if len(DefaultPerUser) != len(kinds) {
		t.Errorf("DefaultPerUser has %d entries, AllKinds has %d — one names a kind the other does not",
			len(DefaultPerUser), len(kinds))
	}
}

// TestAllKindsIsACopy pins that a caller cannot edit the package's idea of what
// exists by writing through the slice it was handed.
func TestAllKindsIsACopy(t *testing.T) {
	got := AllKinds()
	got[0] = "tampered"
	if fresh := AllKinds(); fresh[0] == "tampered" {
		t.Error("AllKinds returns shared backing storage — a caller can rewrite the kind list")
	}
}

func TestKindSpentErrorUnwrapsToPerUserSpent(t *testing.T) {
	for _, k := range AllKinds() {
		t.Run(string(k), func(t *testing.T) {
			var err error = &KindSpentError{Kind: k, Cap: 7}

			// The kind-agnostic test every caller uses.
			if !errors.Is(err, ErrPerUserSpent) {
				t.Errorf("errors.Is(%v, ErrPerUserSpent) = false, want true", err)
			}
			// The two halves must never be confused for each other: they are different
			// news and get different copy.
			if errors.Is(err, ErrGlobalSpent) {
				t.Error("a per-user refusal reports as the global one")
			}

			// And through a wrap, since a consumer module wraps with its own context.
			wrapped := fmt.Errorf("practice: budget: %w", err)
			if !errors.Is(wrapped, ErrPerUserSpent) {
				t.Error("wrapping loses ErrPerUserSpent")
			}
			var got *KindSpentError
			if !errors.As(wrapped, &got) {
				t.Fatal("wrapping loses the *KindSpentError")
			}
			if got.Kind != k || got.Cap != 7 {
				t.Errorf("recovered %+v, want kind %q cap 7", got, k)
			}
			if msg := got.Error(); msg == "" {
				t.Error("Error() is empty")
			}
		})
	}
}

func TestWriteRefusal(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantHandled bool
		wantMsg     string
	}{
		{
			// Every kind now names its own cap: one template, one noun per kind, and no
			// feature that is told "all of your duels" while another is told "all 2".
			// The number is the player's OWN and they could have counted it themselves.
			name:        "duel per-user names the cap",
			err:         &KindSpentError{Kind: KindDuel, Cap: 20, Noun: KindDuel.Noun()},
			wantHandled: true,
			wantMsg:     "you have used all 20 of your duels for today — new ones unlock as the day rolls over",
		},
		{
			name:        "practice per-user names the cap",
			err:         &KindSpentError{Kind: KindPractice, Cap: 20, Noun: KindPractice.Noun()},
			wantHandled: true,
			wantMsg:     "you have used all 20 of your scored drawings for today — new ones unlock as the day rolls over",
		},
		{
			name:        "guess per-user names the cap",
			err:         &KindSpentError{Kind: KindGuess, Cap: 2, Noun: KindGuess.Noun()},
			wantHandled: true,
			wantMsg:     "you have used all 2 of your AI guesses for today — new ones unlock as the day rolls over",
		},
		{
			name:        "assist per-user names the cap",
			err:         &KindSpentError{Kind: KindAssist, Cap: 40, Noun: KindAssist.Noun()},
			wantHandled: true,
			wantMsg:     "you have used all 40 of your AI drawing requests for today — new ones unlock as the day rolls over",
		},
		{
			// A Policy built by hand carries no noun (only Policies fills it), and the
			// message must still read like the configured one: http.go falls back to the
			// same table.
			name:        "no noun falls back to the kind's own word",
			err:         &KindSpentError{Kind: KindGuess, Cap: 2},
			wantHandled: true,
			wantMsg:     "you have used all 2 of your AI guesses for today — new ones unlock as the day rolls over",
		},
		{
			// A kind that shipped before its word still refuses in plain language.
			name:        "unknown kind falls back",
			err:         &KindSpentError{Kind: "inpaint", Cap: 3},
			wantHandled: true,
			wantMsg:     "you have used all 3 of your AI requests for today — new ones unlock as the day rolls over",
		},
		{
			name:        "wrapped per-user refusal",
			err:         fmt.Errorf("game: create: %w", &KindSpentError{Kind: KindDuel, Cap: 20, Noun: KindDuel.Noun()}),
			wantHandled: true,
			wantMsg:     "you have used all 20 of your duels for today — new ones unlock as the day rolls over",
		},
		{
			name:        "bare per-user sentinel",
			err:         ErrPerUserSpent,
			wantHandled: true,
			wantMsg:     "you have used all of your AI requests for today — new ones unlock as the day rolls over",
		},
		{
			// Says nothing about the budget's size or what is left — operator
			// information (docs/API.md §8, §12).
			name:        "global",
			err:         ErrGlobalSpent,
			wantHandled: true,
			wantMsg:     "the AI budget for today is spent — this feature resumes tomorrow",
		},
		{
			name:        "wrapped global",
			err:         fmt.Errorf("practice: %w", ErrGlobalSpent),
			wantHandled: true,
			wantMsg:     "the AI budget for today is spent — this feature resumes tomorrow",
		},
		{name: "unrelated error", err: errors.New("boom"), wantHandled: false},
		{name: "nil", err: nil, wantHandled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handled := WriteRefusal(rec, tt.err)

			if handled != tt.wantHandled {
				t.Fatalf("WriteRefusal handled = %v, want %v", handled, tt.wantHandled)
			}
			if !tt.wantHandled {
				// Nothing may be written: the caller's own switch owns this error, and a
				// header written here would poison its response.
				if rec.Body.Len() != 0 {
					t.Errorf("wrote a body for an unhandled error: %s", rec.Body.String())
				}
				return
			}

			if rec.Code != http.StatusTooManyRequests {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
			}
			// No Retry-After, unlike the per-IP tiers: the window rolls, so there is no
			// fixed reset to name (docs/API.md §3.1).
			if ra := rec.Header().Get("Retry-After"); ra != "" {
				t.Errorf("Retry-After = %q, want none", ra)
			}

			var env struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("decode envelope: %v (body %s)", err, rec.Body.String())
			}
			if env.Error.Code != "rate_limited" {
				t.Errorf("code = %q, want %q", env.Error.Code, "rate_limited")
			}
			if env.Error.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", env.Error.Message, tt.wantMsg)
			}
		})
	}
}

// TestUnbudgetedKindIsNeverCounted is the escape hatch: a kind with no policy, or
// one whose impl is a fake (no provider), must clear without asking the database
// anything. The nil *db.Queries IS the assertion — any read or write would panic.
func TestUnbudgetedKindIsNeverCounted(t *testing.T) {
	b := New(nil, map[Kind]Policy{
		// Configured, generous, but fake-backed: nothing to protect, so nothing to
		// enforce and nothing worth recording.
		KindPractice: {PerUser: 20},
	}, 200, nil)

	for _, tt := range []struct {
		name string
		kind Kind
	}{
		{"no policy at all", KindDuel},
		{"policy with no provider", KindPractice},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			check, spend := b.For(tt.kind)

			if err := check(ctx, "11111111-1111-1111-1111-111111111111"); err != nil {
				t.Errorf("Check = %v, want nil", err)
			}
			if err := spend(ctx, "11111111-1111-1111-1111-111111111111"); err != nil {
				t.Errorf("Spend = %v, want nil", err)
			}
			// The duel's shape: two players granted a round in a transaction the caller
			// owns, and a provider billed separately at each entry into judging. Both
			// ports must skip an unbudgeted kind, or a fake judge would fill the ledger.
			billPlayers, billProvider := b.ForSplit(tt.kind)
			if err := billPlayers(ctx, nil,
				"11111111-1111-1111-1111-111111111111",
				"22222222-2222-2222-2222-222222222222"); err != nil {
				t.Errorf("BillPlayers = %v, want nil", err)
			}
			if err := billProvider(ctx, nil); err != nil {
				t.Errorf("BillProvider = %v, want nil", err)
			}
		})
	}
}

// TestNewCopiesPolicies pins that the ceiling is decided at boot: a caller that
// kept its map cannot move a cap afterwards.
func TestNewCopiesPolicies(t *testing.T) {
	policies := map[Kind]Policy{KindDuel: {Provider: ProviderGoogle, PerUser: 20}}
	b := New(nil, policies, 200, nil)

	policies[KindDuel] = Policy{} // would make the duel unbudgeted if it were shared
	if got := b.policies[KindDuel]; got.Provider != ProviderGoogle || got.PerUser != 20 {
		t.Errorf("policy after caller mutation = %+v, want the one passed to New", got)
	}
}
