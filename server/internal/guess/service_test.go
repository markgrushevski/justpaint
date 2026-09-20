package guess

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"strings"
	"testing"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/judge"
)

// --- stand-ins for the two seams --------------------------------------------
//
// Both are interfaces, so the whole service is exercised with no network, no
// subprocess and no database — which is the point of a module that holds no
// *db.Queries.

type stubRenderer struct {
	png   []byte
	err   error
	calls int
}

func (r *stubRenderer) Render(context.Context, document.Document) ([]byte, error) {
	r.calls++
	return r.png, r.err
}

type stubGuesser struct {
	guess judge.Guess
	err   error
	calls int
	saw   []byte // the raster it was handed
}

func (g *stubGuesser) Guess(_ context.Context, img []byte) (judge.Guess, error) {
	g.calls++
	g.saw = img
	return g.guess, g.err
}

// spendRecorder counts ledger writes and remembers who was billed.
type spendRecorder struct {
	calls int
	users []string
	err   error
}

func (s *spendRecorder) spend(_ context.Context, userIDs ...string) error {
	s.calls++
	s.users = append(s.users, userIDs...)
	return s.err
}

const svcUserID = "11111111-1111-4111-8111-111111111111"

func testLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// tinyPNG is a real, decodable PNG — needed wherever judge.FakeGuesser is the
// guesser, since it actually reads the pixels.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// The renderer ignores the document here, so the zero value is honest about what
// these tests are actually about: the ORDER of the steps around it.
func aDocument() document.Document { return document.Document{} }

// An unconfigured guesser must refuse before it touches anything else. Checking a
// budget for a call that will never be made would spend a read and, worse, teach
// the next reader that the order does not matter.
func TestService_Guess_Unconfigured(t *testing.T) {
	renderer := &stubRenderer{png: tinyPNG(t)}
	budgetCalls := 0
	budget := func(context.Context, string) error { budgetCalls++; return nil }

	svc := NewService(renderer, nil, budget, nil, testLogger())
	_, err := svc.Guess(context.Background(), svcUserID, aDocument())
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
	if budgetCalls != 0 {
		t.Errorf("budget consulted %d times for a call that can never happen", budgetCalls)
	}
	if renderer.calls != 0 {
		t.Errorf("renderer ran %d times with no guesser to send the raster to", renderer.calls)
	}
}

// The ceiling refuses BEFORE the expensive work, and its error reaches the caller
// intact so aibudget.WriteRefusal can still recognise it.
func TestService_Guess_BudgetRefusesBeforeAnyWork(t *testing.T) {
	renderer := &stubRenderer{png: tinyPNG(t)}
	guesser := &stubGuesser{guess: judge.Guess{Label: "a cat", Confidence: 0.9}}
	spend := &spendRecorder{}
	refusal := &aibudget.KindSpentError{Kind: aibudget.KindGuess, Cap: 2}

	svc := NewService(renderer, guesser,
		func(context.Context, string) error { return refusal }, spend.spend, testLogger())

	_, err := svc.Guess(context.Background(), svcUserID, aDocument())
	if !errors.Is(err, aibudget.ErrPerUserSpent) {
		t.Fatalf("err = %v, want the per-user refusal to survive unwrapped", err)
	}
	if renderer.calls != 0 || guesser.calls != 0 || spend.calls != 0 {
		t.Errorf("a refused call still did work: render=%d guess=%d spend=%d",
			renderer.calls, guesser.calls, spend.calls)
	}
}

// The render is OURS and spends no external quota, so a renderer that falls over
// must not cost a player one of the two guesses they get for the day.
func TestService_Guess_RenderFailureIsNotBilled(t *testing.T) {
	renderer := &stubRenderer{err: errors.New("the node worker died")}
	guesser := &stubGuesser{}
	spend := &spendRecorder{}

	svc := NewService(renderer, guesser, nil, spend.spend, testLogger())
	_, err := svc.Guess(context.Background(), svcUserID, aDocument())
	if err == nil {
		t.Fatal("expected the render failure to surface")
	}
	if !strings.Contains(err.Error(), "the node worker died") {
		t.Errorf("err = %v, want it to wrap the renderer's own cause", err)
	}
	if spend.calls != 0 {
		t.Errorf("billed %d calls for a render that never reached a provider", spend.calls)
	}
	if guesser.calls != 0 {
		t.Errorf("the guesser ran %d times with no raster", guesser.calls)
	}
}

// The raster ceiling is checked BEFORE the quota is spent and before anything
// leaves the building: an oversize render is a fault of ours, and the player pays
// for none of it.
func TestService_Guess_RefusesOversizeRaster(t *testing.T) {
	renderer := &stubRenderer{png: make([]byte, maxRasterBytes+1)}
	guesser := &stubGuesser{guess: judge.Guess{Label: "a cat", Confidence: 0.9}}
	spend := &spendRecorder{}

	svc := NewService(renderer, guesser, nil, spend.spend, testLogger())
	_, err := svc.Guess(context.Background(), svcUserID, aDocument())
	if !errors.Is(err, ErrRasterTooLarge) {
		t.Fatalf("err = %v, want ErrRasterTooLarge", err)
	}
	if guesser.calls != 0 {
		t.Errorf("sent an oversize raster to the provider %d times", guesser.calls)
	}
	if spend.calls != 0 {
		t.Errorf("billed %d calls for a raster we refused to send", spend.calls)
	}

	// And the boundary itself is inclusive: exactly at the cap still goes.
	atCap := &stubRenderer{png: make([]byte, maxRasterBytes)}
	if _, err := NewService(atCap, guesser, nil, spend.spend, testLogger()).
		Guess(context.Background(), svcUserID, aDocument()); err != nil {
		t.Errorf("a raster exactly at the cap was refused: %v", err)
	}
}

// The happy path, plus the two facts that matter around it: the guesser is handed
// the SERVER's raster, and the call is billed exactly once, to the caller.
func TestService_Guess_HappyPath(t *testing.T) {
	raster := tinyPNG(t)
	renderer := &stubRenderer{png: raster}
	guesser := &stubGuesser{guess: judge.Guess{
		Label: "a cat wearing a hat", Confidence: 0.82, Alternatives: []string{"a rabbit"},
	}}
	spend := &spendRecorder{}

	svc := NewService(renderer, guesser, nil, spend.spend, testLogger())
	view, err := svc.Guess(context.Background(), svcUserID, aDocument())
	if err != nil {
		t.Fatalf("Guess: %v", err)
	}
	if view.Label != "a cat wearing a hat" || view.Confidence != 0.82 {
		t.Errorf("view = %+v, want the guesser's answer verbatim", view)
	}
	if len(view.Alternatives) != 1 || view.Alternatives[0] != "a rabbit" {
		t.Errorf("alternatives = %v, want the runner-up passed through", view.Alternatives)
	}
	if !bytes.Equal(guesser.saw, raster) {
		t.Error("the guesser was handed something other than the server-rendered raster")
	}
	if spend.calls != 1 || len(spend.users) != 1 || spend.users[0] != svcUserID {
		t.Errorf("billing = %d call(s) to %v, want exactly one to the caller", spend.calls, spend.users)
	}
}

// A call that FAILED still spent the provider's quota. Billing only on success is
// how a broken impl drains a budget that cannot see it.
func TestService_Guess_FailedCallIsStillBilled(t *testing.T) {
	renderer := &stubRenderer{png: tinyPNG(t)}
	guesser := &stubGuesser{err: judge.ErrQuotaExhausted}
	spend := &spendRecorder{}

	svc := NewService(renderer, guesser, nil, spend.spend, testLogger())
	if _, err := svc.Guess(context.Background(), svcUserID, aDocument()); err == nil {
		t.Fatal("expected the guesser's failure to surface")
	}
	if spend.calls != 1 {
		t.Errorf("billed %d calls, want 1 — the request was made and the quota is gone", spend.calls)
	}
}

// The seam is swappable, so its answer is re-checked here. A partial guess must
// never leak past a failed contract check.
func TestService_Guess_RejectsInvalidGuess(t *testing.T) {
	renderer := &stubRenderer{png: tinyPNG(t)}
	guesser := &stubGuesser{guess: judge.Guess{Label: "a cat", Confidence: 42}}

	svc := NewService(renderer, guesser, nil, nil, testLogger())
	view, err := svc.Guess(context.Background(), svcUserID, aDocument())
	if !errors.Is(err, judge.ErrInvalidGuess) {
		t.Fatalf("err = %v, want it to wrap ErrInvalidGuess", err)
	}
	if view.Label != "" || view.Confidence != 0 || view.Alternatives != nil {
		t.Errorf("a rejected answer leaked a partial view: %+v", view)
	}
}

// Nil ports mean unbudgeted, which is what every fake-configured deployment and
// every test gets. The loop must still run end to end.
func TestService_Guess_UnbudgetedRuns(t *testing.T) {
	svc := NewService(&stubRenderer{png: tinyPNG(t)}, judge.NewFakeGuesser(), nil, nil, testLogger())
	view, err := svc.Guess(context.Background(), svcUserID, aDocument())
	if err != nil {
		t.Fatalf("Guess: %v", err)
	}
	if view.Label == "" {
		t.Error("the fake guesser produced no label")
	}
}
