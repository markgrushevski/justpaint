package judge

import (
	"context"
	"fmt"
)

// FakeCritic is the zero-ML default for practice, mirroring FakeJudge: it
// scores by ink coverage — the fraction of non-background pixels — so the
// whole practice loop runs with no API key, quota or network, deterministic
// in the bytes. It never reads the prompt, and the feedback says so: a score
// presented as "how well you drew a fox" when nothing looked for a fox is a
// lie the player cannot detect.
type FakeCritic struct{}

// NewFakeCritic returns the in-process fake critic.
func NewFakeCritic() *FakeCritic { return &FakeCritic{} }

var _ Critic = (*FakeCritic)(nil)

// Critique implements Critic. It ignores the prompt and the context; the result
// honors the contract exactly (a valid score, plain-text feedback within the cap)
// so every consumer path is exercised.
func (FakeCritic) Critique(_ context.Context, req CritiqueRequest) (Critique, error) {
	cov, err := inkCoverage(req.Image)
	if err != nil {
		return Critique{}, fmt.Errorf("decode image: %w", err)
	}
	return Critique{
		Score: cov,
		Feedback: fmt.Sprintf(
			"Your drawing covers %.0f%% of the canvas. This is the fake critic: it measures ink, not meaning, and has not read the prompt — set JUDGE_MODE=gemini for a real critique.",
			cov*100),
	}, nil
}
