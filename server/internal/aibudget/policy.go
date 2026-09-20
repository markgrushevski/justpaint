package aibudget

import (
	"fmt"
	"strings"
)

// Models resolves which model each AI kind runs on: one default for every kind,
// overridden per kind by the operator's AI_MODEL_PER_KIND map (keyed by the kind's
// wire name, exactly like AI_DAILY_PER_USER).
//
// It lives here, beside Policies, because the two answer the same shape of
// question — turning a flat map an operator typed into a per-kind fact the
// composition root can hand to a constructor — and because the valid set of kinds
// lives in this package and grows with the code, which is why config (stdlib only,
// no domain imports) cannot check a kind name and this must.
//
// The kinds differ in how hard their job is, which is the whole reason for the
// knob. The duel judge compares two drawings and its number feeds Elo, so it is
// the one that has to be steadiest; the practice critic scores one drawing against
// a prompt; the guesser only names what it sees and its mistakes cost nothing.
// Running all three on one model means paying the judge's price for the guesser or
// accepting the guesser's steadiness for the judge.
//
// Two things are boot errors rather than shrugs, for the same reason they are in
// Policies: an unknown kind name would configure a model nothing reads, and the
// feature it was meant to pin would quietly keep running on the default and look
// fine. An empty model would build an endpoint with no model id in it, which earns
// a 404 from Google on the first call — hours after the deploy, and only for
// whichever kind was misconfigured.
//
// A kind with no override gets defaultModel, so adding a feature never means
// editing a deployment by hand. Every kind is present in the result.
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

// Policies resolves every AI kind's ceiling: WHOSE quota it spends, and how much
// of it one player may take per rolling day.
//
// It is the only place the two facts meet, and it is a pure function of them so
// the meeting can be tested. providers is what the composition root RESOLVED from
// the impls it actually built — not a second reading of the mode envs. That
// distinction is the whole point: a mode switch says which impl was asked for, an
// impl says whether it really calls anybody, and the two can disagree (an
// ASSIST_MODE=anthropic scaffold that returns an error without any network I/O
// used to be billed for calls it never made). An empty provider is how the budget
// says "never enforced, never recorded".
//
// perUser is the operator's AI_DAILY_PER_USER map, keyed by the kind's wire name;
// a kind they did not name falls back to DefaultPerUser, so adding a feature
// never means editing a deployment by hand.
//
// Three things are boot errors rather than shrugs:
//
//   - an unknown kind name in perUser — the valid set lives here and grows with
//     the code, which is why config (stdlib only, no domain imports) cannot check
//     it and this must. A typo would otherwise configure an allowance nothing
//     reads, and the feature it was meant to bound would run unbudgeted and look
//     fine;
//   - an unknown kind in providers — a wiring slip at the composition root, which
//     would silently leave a real impl unbudgeted;
//   - a kind with a provider and an allowance below 1 — that refuses every call of
//     that kind, which is never what anybody meant by configuring a ceiling.
//
// An allowance for a kind with NO provider is not an error: it is inert, and
// InertAllowances reports it so the boot log can say so.
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
// will ever read, because the impl that was built makes no provider calls. It is
// not an error — a deployment may carry a ceiling for a feature it has switched
// to a fake — but it is worth a word at boot, because a configured ceiling that
// does nothing and an enforced one look identical from the outside.
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
