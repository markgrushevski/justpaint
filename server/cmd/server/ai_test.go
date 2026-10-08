package main

import (
	"testing"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/platform/config"
)

// TestShippedAllowancesBootUnderTheDefaultGlobal pins the two defaults against
// each other: they live in packages that cannot import one another, and a
// default allowance at or above the default global ceiling would refuse to boot
// under the Gemini modes.
func TestShippedAllowancesBootUnderTheDefaultGlobal(t *testing.T) {
	everyKindOnAProvider := make(map[aibudget.Kind]aibudget.Provider)
	for _, kind := range aibudget.AllKinds() {
		everyKindOnAProvider[kind] = aibudget.ProviderGoogle
	}
	if _, err := aibudget.Policies(everyKindOnAProvider, nil, config.DefaultAIDailyGlobal); err != nil {
		t.Fatalf("the shipped defaults do not boot: %v", err)
	}
}
