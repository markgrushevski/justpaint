package document

// ScoredCanvasSize is the square canvas of every drawing a model scores
// (docs/GAME.md §2): duel players are compared on the same canvas, and practice
// scores against the same prompts.
const ScoredCanvasSize = 1080

// ValidateScored is the rule set for a drawing that gets scored, in a duel or in
// practice: the format plus the square ScoredCanvasSize canvas. Free-draw
// features take any canvas the format allows and use ParseAndValidate. Every
// failure is a *ValidationError.
func ValidateScored(data []byte) (Document, error) {
	doc, err := ParseAndValidate(data)
	if err != nil {
		return Document{}, err
	}
	if doc.Width != ScoredCanvasSize || doc.Height != ScoredCanvasSize {
		return Document{}, invalid("submission must be %d×%d", ScoredCanvasSize, ScoredCanvasSize)
	}
	return doc, nil
}
