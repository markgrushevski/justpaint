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
// Node render worker (packages/render/dist/render.mjs), which renders the
// document with the SAME Konva + perfect-freehand projection as the editor — so
// the judged raster matches the editor preview (docs/GAME.md §6, DECISIONS
// "one shared renderer"). The document JSON goes in on stdin; the PNG comes back
// base64 on stdout (base64 keeps the pipe text-safe across platforms).
type NodeRenderer struct {
	nodeBin string // node executable (default "node")
	cliPath string // bundled worker entry (packages/render/dist/render.mjs)
}

// NewNodeRenderer builds the renderer. nodeBin defaults to "node"; cliPath is the
// path to the bundled worker (required — validated at config load).
func NewNodeRenderer(nodeBin, cliPath string) *NodeRenderer {
	if nodeBin == "" {
		nodeBin = "node"
	}
	return &NodeRenderer{nodeBin: nodeBin, cliPath: cliPath}
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

// Render marshals the validated document back to JSON, pipes it to the worker,
// and decodes the base64 PNG. ctx bounds the subprocess (the caller's judging
// timeout applies via exec.CommandContext).
func (r *NodeRenderer) Render(ctx context.Context, doc document.Document) ([]byte, error) {
	docJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("render: marshal document: %w", err)
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
