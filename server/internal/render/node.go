package render

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// NodeRenderer produces the authoritative judged raster by spawning the bundled
// Node render worker (packages/render/dist/render.mjs), which shares the
// editor's Konva + perfect-freehand projection so the judged raster matches the
// preview (docs/GAME.md §6). Document JSON goes in on stdin, a base64 PNG comes
// back on stdout (docs/NOTES.md).
//
// A semaphore, sized by JUDGE_CONCURRENCY, bounds concurrent worker
// subprocesses so a write-rate burst on /api/guess or /api/practice — which
// render inline, outside the judging-pass limiter — can't fork enough of them
// to OOM a small host (docs/NOTES.md).
type NodeRenderer struct {
	nodeBin string // node executable (default "node")
	cliPath string // bundled worker entry (packages/render/dist/render.mjs)
	// slots is a counting semaphore: a token is taken for one subprocess and
	// returned when it exits. A buffered channel avoids a dependency for eight
	// lines; sync.Mutex would only bound N=1.
	slots chan struct{}
}

// NewNodeRenderer builds the renderer. nodeBin defaults to "node"; cliPath is
// the bundled worker's path (required, validated at config load).
//
// concurrency bounds simultaneous worker subprocesses; callers pass the same
// JUDGE_CONCURRENCY that bounds judging passes, since each pass renders twice
// while this counts every process those renders and inline renders share.
// Non-positive is clamped to 1 — a renderer that can run nothing helps nobody.
func NewNodeRenderer(nodeBin, cliPath string, concurrency int) *NodeRenderer {
	if nodeBin == "" {
		nodeBin = "node"
	}
	if concurrency < 1 {
		concurrency = 1
	}
	return &NodeRenderer{nodeBin: nodeBin, cliPath: cliPath, slots: make(chan struct{}, concurrency)}
}

var _ Renderer = (*NodeRenderer)(nil)

// maxWorkerOutput caps the worker's (base64) stdout — defense-in-depth against a
// buggy/replaced worker binary. The honest 1024² PNG is tens of KB; this only
// bounds a runaway. The document input is already DoS-capped upstream.
const maxWorkerOutput = 16 << 20 // 16 MiB

// pngMagic is the 8-byte PNG signature; the worker output must start with it.
var pngMagic = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

// capBuffer accumulates up to cap bytes, then fails the write — so a misbehaving
// worker streaming without bound can't grow memory unchecked (Run surfaces the
// write error).
type capBuffer struct {
	buf bytes.Buffer
	cap int
}

func (w *capBuffer) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.cap {
		return 0, fmt.Errorf("render: worker output exceeds %d bytes", w.cap)
	}
	return w.buf.Write(p)
}

// Render marshals the document to JSON, pipes it to the worker, and decodes the
// base64 PNG. ctx bounds both the subprocess and the wait for a free slot.
//
// It blocks rather than refuses when every slot is busy: every caller already
// passed a bound of its own (the write rate limiter, the judging-concurrency
// limiter), so a slower render beats a lost duel. The wait ends with ctx
// (docs/NOTES.md), and the resulting error names the wait so it isn't mistaken
// for a worker fault.
func (r *NodeRenderer) Render(ctx context.Context, doc document.Document) ([]byte, error) {
	docJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("render: marshal document: %w", err)
	}

	// Taken before the fork and returned after the process exits (cmd.Run waits),
	// so the token covers the memory it is meant to bound.
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	case <-ctx.Done():
		return nil, fmt.Errorf("render: gave up waiting for a worker slot (all %d busy): %w", cap(r.slots), ctx.Err())
	}

	cmd := exec.CommandContext(ctx, r.nodeBin, r.cliPath)
	cmd.Stdin = bytes.NewReader(docJSON)
	stdout := &capBuffer{cap: maxWorkerOutput}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("render: node worker failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	png, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stdout.buf.String()))
	if err != nil {
		return nil, fmt.Errorf("render: decode worker output: %w", err)
	}
	// Guard the seam: a worker that emitted valid base64 of non-PNG bytes should
	// fail here with a clear error, not deep in judging.
	if !bytes.HasPrefix(png, pngMagic) {
		return nil, fmt.Errorf("render: node worker output is not a PNG (%d bytes)", len(png))
	}
	return png, nil
}
