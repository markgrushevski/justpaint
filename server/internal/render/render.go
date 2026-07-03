// Package render turns a validated vector document into the authoritative judged
// raster, rendered off the client (the trust boundary — docs/GAME.md §6,
// DOCUMENT-FORMAT §10). The game loop depends on the Renderer interface, never a
// concrete impl, so the in-process StubRenderer here and a future Konva +
// perfect-freehand Node worker swap by config with no change to the loop —
// exactly like the Judge seam (internal/judge).
package render

import (
	"context"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// JudgeFrameSize is the square judged-raster edge (docs/JUDGE.md §5): 1024×1024,
// opaque white background. Both submissions render to this exact frame so the
// judge compares like for like — the size/background are pinned by the judge
// contract, hence not a per-call option.
const JudgeFrameSize = 1024

// Renderer produces the authoritative PNG for one document.
type Renderer interface {
	Render(ctx context.Context, doc document.Document) ([]byte, error)
}
