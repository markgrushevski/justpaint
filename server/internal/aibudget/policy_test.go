package aibudget

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// TestPolicies is the table this function was moved here to get. It used to be
// `aiPolicies` in package main, where the only way to exercise it was to boot a
// server — which is how it came to hand ASSIST_MODE=anthropic a provider for an
// impl that makes no network call at all.
//
// The cases are the deployments that exist, plus the three that must not boot.
func TestPolicies(t *testing.T) {
	// The four wirings the composition root can produce, named by the mode that
	// produces them. Each is a map from kind to the provider of the impl that was
	// ACTUALLY BUILT — never a second reading of the mode envs, which is the whole
	// point of the move.
	var (
		allFake = map[Kind]Provider{}
		gemini  = map[Kind]Provider{
			KindDuel: ProviderGoogle, KindPractice: ProviderGoogle, KindGuess: ProviderGoogle,
		}
		// JUDGE_MODE=http: the collaborator's service answers one frozen two-image
		// question, so practice and guess have no impl and owe no quota.
		collaborator = map[Kind]Provider{KindDuel: ProviderCollaborator}
	)

	tests := []struct {
		name      string
		providers map[Kind]Provider
		perUser   map[string]int
		want      map[Kind]Policy
		wantErr   string
	}{
		{
			name:      "every impl a fake: nothing is enforced and nothing is recorded",
			providers: allFake,
			want: map[Kind]Policy{
				KindDuel:     {PerUser: 20, Noun: "duels"},
				KindPractice: {PerUser: 20, Noun: "scored drawings"},
				KindGuess:    {PerUser: 2, Noun: "AI guesses"},
				KindAssist:   {PerUser: 40, Noun: "AI drawing requests"},
			},
		},
		{
			name:      "gemini judge, critic and guesser: one provider, the defaults",
			providers: gemini,
			want: map[Kind]Policy{
				KindDuel:     {Provider: ProviderGoogle, PerUser: 20, Noun: "duels"},
				KindPractice: {Provider: ProviderGoogle, PerUser: 20, Noun: "scored drawings"},
				KindGuess:    {Provider: ProviderGoogle, PerUser: 2, Noun: "AI guesses"},
				KindAssist:   {PerUser: 40, Noun: "AI drawing requests"},
			},
		},
		{
			name:      "the collaborator's ML serves duels only",
			providers: collaborator,
			want: map[Kind]Policy{
				KindDuel:     {Provider: ProviderCollaborator, PerUser: 20, Noun: "duels"},
				KindPractice: {PerUser: 20, Noun: "scored drawings"},
				KindGuess:    {PerUser: 2, Noun: "AI guesses"},
				KindAssist:   {PerUser: 40, Noun: "AI drawing requests"},
			},
		},
		{
			// The bug this whole restructure is about, from the budget's side: an impl
			// that calls nobody must arrive here with no provider, and then it is
			// unbudgeted no matter what ASSIST_MODE said.
			name:      "an assist scaffold that calls nobody is unbudgeted",
			providers: map[Kind]Provider{KindDuel: ProviderGoogle},
			perUser:   map[string]int{"assist": 40},
			want: map[Kind]Policy{
				KindDuel:     {Provider: ProviderGoogle, PerUser: 20, Noun: "duels"},
				KindPractice: {PerUser: 20, Noun: "scored drawings"},
				KindGuess:    {PerUser: 2, Noun: "AI guesses"},
				KindAssist:   {PerUser: 40, Noun: "AI drawing requests"},
			},
		},
		{
			name:      "an operator's allowances win over the defaults, per kind",
			providers: gemini,
			perUser:   map[string]int{"duel": 5, "guess": 1},
			want: map[Kind]Policy{
				KindDuel:     {Provider: ProviderGoogle, PerUser: 5, Noun: "duels"},
				KindPractice: {Provider: ProviderGoogle, PerUser: 20, Noun: "scored drawings"},
				KindGuess:    {Provider: ProviderGoogle, PerUser: 1, Noun: "AI guesses"},
				KindAssist:   {PerUser: 40, Noun: "AI drawing requests"},
			},
		},
		{
			// The valid set lives here and grows with the code, which is why config —
			// stdlib only, no domain imports — cannot check it and this must. A typo
			// would otherwise bound nothing while looking configured.
			name:      "an unknown kind in AI_DAILY_PER_USER is a boot error",
			providers: gemini,
			perUser:   map[string]int{"duels": 5},
			wantErr:   `unknown kind "duels"`,
		},
		{
			name:      "an unknown kind in the provider map is a boot error",
			providers: map[Kind]Provider{"inpaint": ProviderGoogle},
			wantErr:   `no such kind "inpaint"`,
		},
		{
			// A real provider with a zero allowance refuses every call of that kind,
			// silently, forever. config rejects a configured 0; this catches the other
			// road to one — a kind whose DefaultPerUser entry was never added.
			name:      "a provider with a zero allowance refuses everything, so it must not boot",
			providers: gemini,
			perUser:   map[string]int{"duel": 0},
			wantErr:   "refuses every call",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Policies(tt.providers, tt.perUser)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Policies = %+v, want an error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("a refused config still returned policies: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Policies: %v", err)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("Policies =\n\t%+v\nwant\n\t%+v", got, tt.want)
			}
		})
	}
}

// TestPoliciesCoversEveryKind pins that a kind nobody wired still gets a policy
// rather than being absent: absent and provider-less behave identically today,
// and a reader should not have to know that to be sure a new feature is off by
// default rather than unbounded.
func TestPoliciesCoversEveryKind(t *testing.T) {
	got, err := Policies(nil, nil)
	if err != nil {
		t.Fatalf("Policies: %v", err)
	}
	for _, kind := range AllKinds() {
		p, ok := got[kind]
		if !ok {
			t.Errorf("no policy for %q", kind)
			continue
		}
		if p.Provider != "" {
			t.Errorf("%q got provider %q with nothing wired", kind, p.Provider)
		}
		if p.PerUser != DefaultPerUser[kind] {
			t.Errorf("%q per-user = %d, want the default %d", kind, p.PerUser, DefaultPerUser[kind])
		}
	}
}

// TestInertAllowances: a ceiling configured for a feature that calls nobody reads
// exactly like an enforced one from the outside, so boot says which is which.
func TestInertAllowances(t *testing.T) {
	tests := []struct {
		name      string
		providers map[Kind]Provider
		perUser   map[string]int
		want      []Kind
	}{
		{
			name:      "nothing configured, nothing to say",
			providers: map[Kind]Provider{KindDuel: ProviderGoogle},
		},
		{
			name:      "an allowance for a kind with a provider is not inert",
			providers: map[Kind]Provider{KindDuel: ProviderGoogle},
			perUser:   map[string]int{"duel": 5},
		},
		{
			// The ASSIST_MODE=anthropic-scaffold deployment: the operator set a number
			// and nothing will ever read it.
			name:      "an allowance for a kind with no provider is inert",
			providers: map[Kind]Provider{KindDuel: ProviderGoogle},
			perUser:   map[string]int{"duel": 5, "assist": 40},
			want:      []Kind{KindAssist},
		},
		{
			// Stable order (AllKinds), so two boots are diffable.
			name:      "several, in declaration order",
			providers: nil,
			perUser:   map[string]int{"assist": 40, "duel": 5, "guess": 2},
			want:      []Kind{KindDuel, KindGuess, KindAssist},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policies, err := Policies(tt.providers, tt.perUser)
			if err != nil {
				t.Fatalf("Policies: %v", err)
			}
			if got := InertAllowances(policies, tt.perUser); !slices.Equal(got, tt.want) {
				t.Errorf("InertAllowances = %v, want %v", got, tt.want)
			}
		})
	}
}
