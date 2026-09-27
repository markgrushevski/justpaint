package aibudget

import (
	"fmt"
	"strings"
)

// Models resolves which model each AI kind runs on: one default for every
// kind, overridden per kind by the operator's AI_MODEL_PER_KIND map (keyed by
// the kind's wire name, exactly like AI_DAILY_PER_USER). It lives beside
// Policies for the same reason: turning a flat operator map into a per-kind
// fact, checked against the valid set of kinds that lives in this package —
// which is why config (stdlib only, no domain imports) cannot check it and
// this must.
//
// The kinds differ in how hard their job is, which is the whole reason for
// the knob: the duel judge's number feeds Elo and has to be steadiest, while
// the guesser's mistakes cost nothing.
//
// Boot errors, not shrugs: an unknown kind name would configure a model
// nothing reads; an empty model builds an endpoint with no model id, which
// earns a 404 from Google on the first call, hours after the deploy.
//
// A kind with no override gets defaultModel. Every kind is present in the
// result.
func Models(defaultModel string, perKind map[string]string) (map[Kind]string, error) {
	for name, model := range perKind {
		if _, ok := ParseKind(name); !ok {
			return nil, fmt.Errorf("config: AI_MODEL_PER_KIND names an unknown kind %q; valid kinds are %v", name, AllKinds())
		}
		if strings.TrimSpace(model) == "" {
			return nil, fmt.Errorf("config: AI_MODEL_PER_KIND names no model for kind %q", name)
		}
	}

	models := make(map[Kind]string, len(AllKinds()))
	for _, kind := range AllKinds() {
		model := defaultModel
		if override, ok := perKind[string(kind)]; ok {
			model = override
		}
		models[kind] = model
	}
	return models, nil
}

// Policies resolves every AI kind's ceiling: whose quota it spends, and how
// much of it one player may take per rolling day. It is the only place the
// two facts meet, and it is a pure function of them so the meeting can be
// tested.
//
// providers must be what the composition root RESOLVED from the impls it
// actually built, never a second reading of the mode envs: a mode says which
// impl was asked for, an impl says whether it really calls anybody, and an
// empty provider is how the budget says "never enforced, never recorded".
//
// perUser is the operator's AI_DAILY_PER_USER map, keyed by the kind's wire
// name; a kind they did not name falls back to DefaultPerUser.
//
// Three boot errors: an unknown kind name in perUser or in providers (the
// valid set lives here, not in config, and grows with the code), and a kind
// with a provider whose allowance is below 1, which would refuse every call
// of that kind. An allowance for a kind with no provider is not an error —
// see InertAllowances.
func Policies(providers map[Kind]Provider, perUser map[string]int) (map[Kind]Policy, error) {
	for name := range perUser {
		if _, ok := ParseKind(name); !ok {
			return nil, fmt.Errorf("config: AI_DAILY_PER_USER names an unknown kind %q; valid kinds are %v", name, AllKinds())
		}
	}
	for kind := range providers {
		if _, ok := ParseKind(string(kind)); !ok {
			return nil, fmt.Errorf("aibudget: no such kind %q has a provider; valid kinds are %v", kind, AllKinds())
		}
	}

	policies := make(map[Kind]Policy, len(AllKinds()))
	for _, kind := range AllKinds() {
		allowance, ok := perUser[string(kind)]
		if !ok {
			allowance = DefaultPerUser[kind]
		}
		provider := providers[kind]
		if provider != "" && allowance < 1 {
			return nil, fmt.Errorf("config: %s is served by %s but its per-user allowance is %d, which refuses every call; set AI_DAILY_PER_USER=%s=<n>",
				kind, provider, allowance, kind)
		}
		policies[kind] = Policy{Provider: provider, PerUser: allowance, Noun: kind.Noun()}
	}
	return policies, nil
}

// InertAllowances names the kinds the operator gave an allowance that nothing
// will read, because the impl built for that kind makes no provider calls.
// Not an error — a deployment may carry a ceiling for a feature switched to a
// fake — but worth a word at boot, since a configured ceiling that does
// nothing looks identical to an enforced one from the outside.
//
// Returned in AllKinds order so two boots are diffable.
func InertAllowances(policies map[Kind]Policy, perUser map[string]int) []Kind {
	var inert []Kind
	for _, kind := range AllKinds() {
		if _, named := perUser[string(kind)]; !named {
			continue
		}
		if policies[kind].Provider == "" {
			inert = append(inert, kind)
		}
	}
	return inert
}
