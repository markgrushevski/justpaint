// Package judge is the seam to the external ML judge (docs/JUDGE.md — the single
// source of truth for this contract). The game module depends on the Judge
// interface, never a concrete impl, so the in-process FakeJudge and a future
// HTTPJudge swap by config with no change to the game loop. The judge scores
// pre-rendered PNGs and never sees our vector document (trust boundary,
// DOCUMENT-FORMAT §10); A/B are positional and mapped to player ids by game.
package judge

import (
	"context"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

// Positional verdict values (JUDGE.md §3) — NOT player ids.
const (
	WinnerA   = "A"
	WinnerB   = "B"
	WinnerTie = "tie"
)

// maxReasonLen bounds the player-facing rationale (JUDGE.md §2).
const maxReasonLen = 500

// Judge scores two authoritative PNGs against a prompt (JUDGE.md §7).
type Judge interface {
	Score(ctx context.Context, req Request) (Result, error)
}

// Request carries the prompt + two pre-rendered PNGs (square 1024², opaque
// background — §5). Bytes regardless of wire mode; A/B are positional, assigned
// by the game module, which remembers the mapping.
type Request struct {
	Prompt string
	ImageA []byte
	ImageB []byte
}

// Result is the judge's verdict (JUDGE.md §2). Winner is positional; scores are
// absolute similarity-to-prompt in [0,1] (higher = better), independent (need
// not sum to 1).
type Result struct {
	ScoreA float64
	ScoreB float64
	Winner string
	Reason string
}

// ErrInvalidResult marks a result that violates the JUDGE.md §2 contract.
var ErrInvalidResult = errors.New("invalid judge result")

// Validate enforces the §2 contract: finite scores in [0,1], a winner in the
// enum, a reason within the length cap. The HTTPJudge uses this to reject a
// malformed 200 from the collaborator (a contract violation, never a verdict).
func (r Result) Validate() error {
	if err := validScore("scoreA", r.ScoreA); err != nil {
		return err
	}
	if err := validScore("scoreB", r.ScoreB); err != nil {
		return err
	}
	switch r.Winner {
	case WinnerA, WinnerB, WinnerTie:
	default:
		return fmt.Errorf("%w: winner %q not in {A,B,tie}", ErrInvalidResult, r.Winner)
	}
	if utf8.RuneCountInString(r.Reason) > maxReasonLen {
		return fmt.Errorf("%w: reason exceeds %d chars", ErrInvalidResult, maxReasonLen)
	}
	return nil
}

func validScore(name string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
		return fmt.Errorf("%w: %s %v not in [0,1]", ErrInvalidResult, name, v)
	}
	return nil
}
