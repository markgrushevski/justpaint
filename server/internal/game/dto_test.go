package game

import "testing"

func strptr(s string) *string { return &s }

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
		m := buildMatchDTO(roster(statusOpen, nil, nil), me)
		if m.Prompt.ID != promptID {
			t.Errorf("prompt id = %q, want %q", m.Prompt.ID, promptID)
		}
		if m.Prompt.Text != nil {
			t.Errorf("prompt text = %q, want nil (redacted while open)", *m.Prompt.Text)
		}
	})

	t.Run("drawing reveals the prompt text to both", func(t *testing.T) {
		m := buildMatchDTO(roster(statusDrawing, nil, nil), me)
		if m.Prompt.Text == nil || *m.Prompt.Text != promptTx {
			t.Errorf("prompt text = %v, want %q once drawing", m.Prompt.Text, promptTx)
		}
	})

	t.Run("opponent drawingId hidden until done; own is visible", func(t *testing.T) {
		// Both have submitted; match still `drawing` (waiting on judge is `judging`,
		// but the redaction rule is identical for any non-`done` state).
		m := buildMatchDTO(roster(statusDrawing, strptr(myDraw), strptr(oppDraw)), me)

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
		m := buildMatchDTO(roster(statusDone, strptr(myDraw), strptr(oppDraw)), me)
		theirs := playerByID(m, opp)
		if theirs.DrawingID == nil || *theirs.DrawingID != oppDraw {
			t.Errorf("opponent drawingId = %v, want %q once done", theirs.DrawingID, oppDraw)
		}
	})

	t.Run("redaction is symmetric — opponent viewer hides my drawing", func(t *testing.T) {
		// Same match, rendered for the opponent: now MY drawing is the secret one.
		m := buildMatchDTO(roster(statusDrawing, strptr(myDraw), strptr(oppDraw)), opp)
		mineToThem := playerByID(m, me)
		if mineToThem.DrawingID != nil {
			t.Errorf("my drawingId leaked to opponent = %q, want nil", *mineToThem.DrawingID)
		}
		if !mineToThem.Submitted {
			t.Error("my submitted flag hidden from opponent, want visible")
		}
	})

	t.Run("canvas echoes the canonical game size", func(t *testing.T) {
		m := buildMatchDTO(roster(statusOpen, nil, nil), me)
		if m.Canvas.Width != GameCanvasSize || m.Canvas.Height != GameCanvasSize {
			t.Errorf("canvas = %dx%d, want %dx%d", m.Canvas.Width, m.Canvas.Height, GameCanvasSize, GameCanvasSize)
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
