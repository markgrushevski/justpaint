package game

import (
	"testing"
	"time"
)

func strptr(s string) *string { return &s }

// fixedNow is a stable response-build instant for the pure buildMatchDTO tests.
var fixedNow = time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

// TestBuildMatchDTO_Redaction pins the two visibility rules (docs/GAME.md §4.2,
// §5) that buildMatchDTO enforces, from each viewer's perspective.
func TestBuildMatchDTO_Redaction(t *testing.T) {
	const (
		me       = "user-me"
		opp      = "user-opp"
		myDraw   = "draw-me"
		oppDraw  = "draw-opp"
		promptID = "prompt-1"
		promptTx = "a fox riding a bicycle"
	)

	// roster builds a two-player view in a given status with optional submissions.
	roster := func(status string, myDrawing, oppDrawing *string) MatchView {
		return MatchView{
			ID:         "match-1",
			Mode:       modeAsync,
			Status:     status,
			PromptID:   promptID,
			PromptText: promptTx,
			Players: []PlayerRow{
				{UserID: me, DisplayName: strptr("Ada"), DrawingID: myDrawing},
				{UserID: opp, DisplayName: strptr("Bo"), DrawingID: oppDrawing},
			},
		}
	}

	// playerByID finds a player DTO in the rendered match (order-independent).
	playerByID := func(m matchDTO, id string) playerDTO {
		t.Helper()
		for _, p := range m.Players {
			if p.UserID == id {
				return p
			}
		}
		t.Fatalf("player %s not in rendered match", id)
		return playerDTO{}
	}

	t.Run("open hides the prompt text (no pre-drawing)", func(t *testing.T) {
		// A lone creator waiting in `open` must not see the prompt yet.
		m := buildMatchDTO(roster(statusOpen, nil, nil), me, fixedNow)
		if m.Prompt.ID != promptID {
			t.Errorf("prompt id = %q, want %q", m.Prompt.ID, promptID)
		}
		if m.Prompt.Text != nil {
			t.Errorf("prompt text = %q, want nil (redacted while open)", *m.Prompt.Text)
		}
	})

	t.Run("drawing reveals the prompt text to both", func(t *testing.T) {
		m := buildMatchDTO(roster(statusDrawing, nil, nil), me, fixedNow)
		if m.Prompt.Text == nil || *m.Prompt.Text != promptTx {
			t.Errorf("prompt text = %v, want %q once drawing", m.Prompt.Text, promptTx)
		}
	})

	t.Run("opponent drawingId hidden until done; own is visible", func(t *testing.T) {
		// Both have submitted; match still `drawing` (waiting on judge is `judging`,
		// but the redaction rule is identical for any non-`done` state).
		m := buildMatchDTO(roster(statusDrawing, strptr(myDraw), strptr(oppDraw)), me, fixedNow)

		mine := playerByID(m, me)
		if !mine.Submitted {
			t.Error("own submitted = false, want true")
		}
		if mine.DrawingID == nil || *mine.DrawingID != myDraw {
			t.Errorf("own drawingId = %v, want %q", mine.DrawingID, myDraw)
		}

		theirs := playerByID(m, opp)
		if !theirs.Submitted {
			t.Error("opponent submitted flag = false, want true (the flag is not secret)")
		}
		if theirs.DrawingID != nil {
			t.Errorf("opponent drawingId = %q, want nil (redacted until done)", *theirs.DrawingID)
		}
	})

	t.Run("done reveals the opponent drawingId", func(t *testing.T) {
		m := buildMatchDTO(roster(statusDone, strptr(myDraw), strptr(oppDraw)), me, fixedNow)
		theirs := playerByID(m, opp)
		if theirs.DrawingID == nil || *theirs.DrawingID != oppDraw {
			t.Errorf("opponent drawingId = %v, want %q once done", theirs.DrawingID, oppDraw)
		}
	})

	t.Run("redaction is symmetric — opponent viewer hides my drawing", func(t *testing.T) {
		// Same match, rendered for the opponent: now MY drawing is the secret one.
		m := buildMatchDTO(roster(statusDrawing, strptr(myDraw), strptr(oppDraw)), opp, fixedNow)
		mineToThem := playerByID(m, me)
		if mineToThem.DrawingID != nil {
			t.Errorf("my drawingId leaked to opponent = %q, want nil", *mineToThem.DrawingID)
		}
		if !mineToThem.Submitted {
			t.Error("my submitted flag hidden from opponent, want visible")
		}
	})

	t.Run("canvas echoes the canonical game size", func(t *testing.T) {
		m := buildMatchDTO(roster(statusOpen, nil, nil), me, fixedNow)
		if m.Canvas.Width != GameCanvasSize || m.Canvas.Height != GameCanvasSize {
			t.Errorf("canvas = %dx%d, want %dx%d", m.Canvas.Width, m.Canvas.Height, GameCanvasSize, GameCanvasSize)
		}
	})

	t.Run("serverTime always present; deadline nil while open, RFC3339Nano once set", func(t *testing.T) {
		wantNow := fixedNow.UTC().Format(time.RFC3339Nano)

		// open → no deadline yet.
		open := buildMatchDTO(roster(statusOpen, nil, nil), me, fixedNow)
		if open.ServerTime != wantNow {
			t.Errorf("serverTime = %q, want %q", open.ServerTime, wantNow)
		}
		if open.DrawingDeadline != nil {
			t.Errorf("drawingDeadline = %q, want nil while open", *open.DrawingDeadline)
		}

		// drawing → deadline formatted as RFC3339Nano UTC.
		deadline := time.Date(2026, 7, 11, 12, 1, 30, 0, time.UTC)
		v := roster(statusDrawing, nil, nil)
		v.DrawingDeadline = &deadline
		drawing := buildMatchDTO(v, me, fixedNow)
		if drawing.DrawingDeadline == nil || *drawing.DrawingDeadline != deadline.Format(time.RFC3339Nano) {
			t.Errorf("drawingDeadline = %v, want %q", drawing.DrawingDeadline, deadline.Format(time.RFC3339Nano))
		}
	})
}

// TestBuildResultDTO pins the two shapes the result endpoint returns (docs/API.md
// §8.4): a compact pending body until done, the full verdict once done.
func TestBuildResultDTO(t *testing.T) {
	t.Run("in-flight → pending shape, no verdict fields", func(t *testing.T) {
		dto := buildResultDTO(ResultView{Status: statusJudging, Ready: false})
		p, ok := dto.(resultPending)
		if !ok {
			t.Fatalf("want resultPending, got %T", dto)
		}
		if p.Status != statusJudging || p.Ready {
			t.Errorf("pending = %+v, want {judging, false}", p)
		}
	})

	score := 0.8
	before, after := int32(1200), int32(1216)
	won := "user-me"

	t.Run("done with a winner → full verdict, isTie false", func(t *testing.T) {
		reason := "A wins"
		dto := buildResultDTO(ResultView{
			Status: statusDone, Ready: true,
			PromptID: "p1", PromptText: "a fox riding a bicycle",
			WinnerUserID: &won, Reason: &reason,
			Players: []ResultPlayer{{
				UserID: won, DisplayName: strptr("Ada"), DrawingID: strptr("d1"),
				Score: &score, RatingBefore: &before, RatingAfter: &after,
			}},
		})
		d, ok := dto.(resultDone)
		if !ok {
			t.Fatalf("want resultDone, got %T", dto)
		}
		if !d.Ready || d.IsTie {
			t.Errorf("ready=%v isTie=%v, want true/false", d.Ready, d.IsTie)
		}
		if d.Prompt.Text == nil || *d.Prompt.Text != "a fox riding a bicycle" {
			t.Errorf("prompt text = %v, want revealed once done", d.Prompt.Text)
		}
		if d.WinnerUserID == nil || *d.WinnerUserID != won {
			t.Errorf("winner = %v, want %q", d.WinnerUserID, won)
		}
		if len(d.Players) != 1 || d.Players[0].JudgedImageURL != nil {
			t.Errorf("judgedImageUrl must be nil until storage+render land, got %+v", d.Players)
		}
		// Nil resolution (legacy pre-migration done row) defaults to 'judged'.
		if d.Resolution != resolutionJudged {
			t.Errorf("resolution = %q, want %q when the view's is nil", d.Resolution, resolutionJudged)
		}
	})

	t.Run("forfeit resolution passes through", func(t *testing.T) {
		forfeit := resolutionForfeit
		dto := buildResultDTO(ResultView{
			Status: statusDone, Ready: true,
			WinnerUserID: &won, Resolution: &forfeit,
		})
		d := dto.(resultDone)
		if d.Resolution != resolutionForfeit {
			t.Errorf("resolution = %q, want %q", d.Resolution, resolutionForfeit)
		}
		if d.IsTie {
			t.Error("forfeit has a winner, isTie must be false")
		}
	})

	t.Run("done tie → winnerUserId nil, isTie true", func(t *testing.T) {
		dto := buildResultDTO(ResultView{Status: statusDone, Ready: true, WinnerUserID: nil})
		d := dto.(resultDone)
		if d.WinnerUserID != nil || !d.IsTie {
			t.Errorf("tie: winner=%v isTie=%v, want nil/true", d.WinnerUserID, d.IsTie)
		}
	})
}

// TestIsPlayer covers the ownership gate Get uses to hide foreign matches.
func TestIsPlayer(t *testing.T) {
	players := []PlayerRow{{UserID: "a"}, {UserID: "b"}}
	if !isPlayer(players, "b") {
		t.Error("isPlayer(b) = false, want true")
	}
	if isPlayer(players, "c") {
		t.Error("isPlayer(c) = true, want false (non-player must be hidden)")
	}
	if isPlayer(nil, "a") {
		t.Error("isPlayer(nil,...) = true, want false")
	}
}
