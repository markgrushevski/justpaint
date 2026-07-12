package game

import "testing"

// recordingPublisher captures publish calls so a test can assert what was published
// (and, for SetPublisher, that a real publisher is installed as-is).
type recordingPublisher struct {
	matchChanged []string
	submitted    [][2]string
	judging      []string
	resolved     []string
	abandoned    []string
}

func (r *recordingPublisher) MatchChanged(m string) { r.matchChanged = append(r.matchChanged, m) }
func (r *recordingPublisher) PlayerSubmitted(m, u string) {
	r.submitted = append(r.submitted, [2]string{m, u})
}
func (r *recordingPublisher) Judging(m string)   { r.judging = append(r.judging, m) }
func (r *recordingPublisher) Resolved(m string)  { r.resolved = append(r.resolved, m) }
func (r *recordingPublisher) Abandoned(m string) { r.abandoned = append(r.abandoned, m) }

// TestNewServiceDefaultsToNopPublisher proves the seam is inert by default: NewService
// installs NopPublisher, its no-op methods are safe to call with no hub wired (so the
// whole round-deadline suite runs unchanged), SetPublisher(nil) resets to the no-op, and
// SetPublisher installs a real publisher as-is.
func TestNewServiceDefaultsToNopPublisher(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil)
	if _, ok := svc.publisher.(NopPublisher); !ok {
		t.Fatalf("NewService publisher = %T, want NopPublisher", svc.publisher)
	}

	// The no-op tail every committed path runs must not panic with no hub.
	svc.publisher.MatchChanged("m")
	svc.publisher.PlayerSubmitted("m", "u")
	svc.publisher.Judging("m")
	svc.publisher.Resolved("m")
	svc.publisher.Abandoned("m")
	svc.publishOutcome("m", outcomeForfeit)
	svc.publishOutcome("m", outcomeAbandoned)
	svc.publishOutcome("m", outcomeJudging)
	svc.publishOutcome("m", outcomeNone)

	svc.SetPublisher(nil)
	if _, ok := svc.publisher.(NopPublisher); !ok {
		t.Fatalf("SetPublisher(nil) publisher = %T, want NopPublisher", svc.publisher)
	}

	rec := &recordingPublisher{}
	svc.SetPublisher(rec)
	// publishOutcome must map outcomes to the right calls.
	svc.publishOutcome("m1", outcomeForfeit)
	svc.publishOutcome("m2", outcomeAbandoned)
	svc.publishOutcome("m3", outcomeJudging)
	svc.publishOutcome("m4", outcomeNone) // no-op
	if len(rec.resolved) != 1 || rec.resolved[0] != "m1" {
		t.Fatalf("outcomeForfeit → resolved, got %v", rec.resolved)
	}
	if len(rec.abandoned) != 1 || rec.abandoned[0] != "m2" {
		t.Fatalf("outcomeAbandoned → abandoned, got %v", rec.abandoned)
	}
	if len(rec.judging) != 1 || rec.judging[0] != "m3" {
		t.Fatalf("outcomeJudging → judging, got %v", rec.judging)
	}
}
