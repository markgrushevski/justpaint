package aibudget

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// TestPolicies covers the deployments that exist, plus the three configs
// that must not boot.
func TestPolicies(t *testing.T) {
	// The four wirings the composition root can produce, named by the mode that
	// produces them. Each is a map from kind to the provider of the impl that
	// was actually built — never a second reading of the mode envs.
	var (
		allFake = map[Kind]Provider{}
		gemini  = map[Kind]Provider{
			KindDuel: ProviderGoogle, KindPractice: ProviderGoogle, KindGuess: ProviderGoogle,
		}
		// JUDGE_MODE=http: the external judge service answers one frozen two-image
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
			name:      "the external ML judge serves duels only",
			providers: collaborator,
			want: map[Kind]Policy{
				KindDuel:     {Provider: ProviderCollaborator, PerUser: 20, Noun: "duels"},
				KindPractice: {PerUser: 20, Noun: "scored drawings"},
				KindGuess:    {PerUser: 2, Noun: "AI guesses"},
				KindAssist:   {PerUser: 40, Noun: "AI drawing requests"},
			},
		},
		{
			// An impl that calls nobody must arrive here with no provider, and then
			// it is unbudgeted no matter what ASSIST_MODE said.
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
			// The valid set lives here, not in config (stdlib only), and a typo would
			// otherwise bound nothing while looking configured.
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
			// config rejects a configured 0; this catches the other road to one — a
			// kind whose DefaultPerUser entry was never added.
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

// TestPoliciesCoversEveryKind pins that a kind nobody wired still gets a
// policy rather than being absent, so a new feature reads as off by default
// rather than unbounded.
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

// TestModels pins one default for every kind, overridden per kind, and a typo
// refused at boot rather than silently leaving a kind on the default.
func TestModels(t *testing.T) {
	const def = "gemini-3.6-flash"

	tests := []struct {
		name    string
		perKind map[string]string
		want    map[Kind]string
		wantErr string
	}{
		{
			name: "nothing configured: every kind on the default",
			want: map[Kind]string{
				KindDuel: def, KindPractice: def, KindGuess: def, KindAssist: def,
			},
		},
		{
			// The point of the knob: the duel's number feeds Elo, the guesser's
			// mistakes cost nothing, so they need not share a model.
			name:    "an override wins for its kind and leaves the rest alone",
			perKind: map[string]string{"duel": "gemini-3.6-pro", "guess": "gemini-3.6-flash-lite"},
			want: map[Kind]string{
				KindDuel: "gemini-3.6-pro", KindPractice: def,
				KindGuess: "gemini-3.6-flash-lite", KindAssist: def,
			},
		},
		{
			name:    "every kind overridden",
			perKind: map[string]string{"duel": "a", "practice": "b", "guess": "c", "assist": "d"},
			want: map[Kind]string{
				KindDuel: "a", KindPractice: "b", KindGuess: "c", KindAssist: "d",
			},
		},
		{
			// Same rule as the unknown kind in AI_DAILY_PER_USER: the valid set lives
			// here, not in config, so a typo would otherwise pin nothing while
			// looking configured.
			name:    "an unknown kind is a boot error naming the valid set",
			perKind: map[string]string{"duels": "gemini-3.6-pro"},
			wantErr: `unknown kind "duels"`,
		},
		{
			// An empty model builds an endpoint with no model id, which earns a 404
			// from Google on the first call, hours after the deploy.
			name:    "an empty model is a boot error",
			perKind: map[string]string{"duel": "   "},
			wantErr: "names no model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Models(def, tt.perKind)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Models = %+v, want an error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("a refused config still returned models: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Models: %v", err)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("Models =\n\t%+v\nwant\n\t%+v", got, tt.want)
			}
		})
	}
}

// TestProviderWithModel pins the key the global ceiling is counted on. Google
// meters its free tier per model, so two kinds on two models are two quota
// pools, not one shared "google" counter.
func TestProviderWithModel(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		model    string
		want     Provider
	}{
		{
			name:     "a metered vendor carries its model",
			provider: ProviderGoogle,
			model:    "gemini-3.6-flash",
			want:     "google:gemini-3.6-flash",
		},
		{
			name:     "two models are two different keys",
			provider: ProviderGoogle,
			model:    "gemini-3.6-pro",
			want:     "google:gemini-3.6-pro",
		},
		{
			// The external ML judge's service is one service however many models sit
			// behind it, so it is never narrowed.
			name:     "an empty model leaves the provider bare",
			provider: ProviderCollaborator,
			want:     ProviderCollaborator,
		},
		{
			// "no provider" means unbudgeted, and must survive a WithModel called on
			// a mode that turned out not to call anybody.
			name:  "no provider stays no provider",
			model: "gemini-3.6-flash",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.WithModel(tt.model); got != tt.want {
				t.Errorf("%q.WithModel(%q) = %q, want %q", tt.provider, tt.model, got, tt.want)
			}
		})
	}
}

// TestModelScopedProviderIsABudgetedProvider pins that a model-scoped key is
// not a special case anywhere in the budget: it must survive Policies as an
// ordinary provider, enforced like any other.
func TestModelScopedProviderIsABudgetedProvider(t *testing.T) {
	flash := ProviderGoogle.WithModel("gemini-3.6-flash")
	pro := ProviderGoogle.WithModel("gemini-3.6-pro")

	policies, err := Policies(map[Kind]Provider{KindDuel: pro, KindGuess: flash}, nil)
	if err != nil {
		t.Fatalf("Policies: %v", err)
	}
	if got := policies[KindDuel].Provider; got != pro {
		t.Errorf("duel provider = %q, want %q", got, pro)
	}
	if got := policies[KindGuess].Provider; got != flash {
		t.Errorf("guess provider = %q, want %q", got, flash)
	}
	// The two kinds must not collapse into one pool.
	if policies[KindDuel].Provider == policies[KindGuess].Provider {
		t.Error("two kinds on two models share one provider key — their quota pools would be conflated")
	}
	// A model-scoped provider is still a real provider: nothing about it may
	// read as unbudgeted.
	if InertAllowances(policies, map[string]int{"duel": 5}) != nil {
		t.Error("a model-scoped provider was treated as no provider at all")
	}
}

// TestInertAllowances pins that a ceiling configured for a feature that calls
// nobody reads exactly like an enforced one from the outside, so boot says
// which is which.
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
			// ASSIST_MODE=fake: the operator set a number and nothing reads it,
			// because the impl that was built calls nobody.
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
