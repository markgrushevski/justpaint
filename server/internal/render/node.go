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
//
// Every Render call spawns a node-canvas process, which is tens of megabytes of
// resident memory, so the number of them in flight at once is the one thing about
// this renderer that can take down the whole binary on a 512 MB host. The bound
// lives HERE rather than at a caller, because it has to hold across callers:
// internal/game bounds its own judging passes, but /api/guess and /api/practice
// render inline on the request goroutine, so one IP's burst under the write
// rate-limit tier (30 in 2s) is 30 concurrent subprocesses with nothing between
// them and the OOM killer. A limiter inside the renderer is inherited by every
// caller, present and future, including the ones nobody has written yet.
type NodeRenderer struct {
	nodeBin string // node executable (default "node")
	cliPath string // bundled worker entry (packages/render/dist/render.mjs)
	// slots is a counting semaphore: a token is taken for the duration of one
	// subprocess and returned after it exits. Buffered channel rather than
	// golang.org/x/sync/semaphore because that is a dependency for eight lines, and
	// rather than sync.Mutex because the bound is N, not one.
	slots chan struct{}
}

// NewNodeRenderer builds the renderer. nodeBin defaults to "node"; cliPath is the
// path to the bundled worker (required — validated at config load).
//
// concurrency is the ceiling on simultaneous worker subprocesses. It is the same
// number as the judging bound (JUDGE_CONCURRENCY, config.DefaultJudgeConcurrency)
// because it bounds the same scarce thing from the other side — that one counts
// judging PASSES, each of which renders twice in sequence, while this one counts
// the processes those renders and every inline render share. A non-positive value
// is clamped to 1: a renderer that can run nothing is not a safer renderer.
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

// Render marshals the validated document back to JSON, pipes it to the worker,
// and decodes the base64 PNG. ctx bounds the subprocess (the caller's judging
// timeout applies via exec.CommandContext) and the wait for a free slot.
//
// It BLOCKS while every slot is busy rather than refusing. Every caller already
// arrived through a bound of its own — the per-IP write rate-limit tier on the
// inline endpoints, the judging-concurrency limiter on the duel — so the queue in
// front of this is short and made of work somebody already said yes to, and the
// honest answer to "the machine is busy" is a slower render rather than a lost
// duel. The wait is not unbounded either: it ends when the caller's own context
// does (game.JudgePassBudget, practice/guess RunBudget), and the resulting error
// names the wait so it is not mistaken for a worker fault.
func (r *NodeRenderer) Render(ctx context.Context, doc document.Document) ([]byte, error) {
	docJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("render: marshal document: %w", err)
	}

	// Taken before the fork and returned after the process has exited (cmd.Run
	// waits), so the token really does cover the memory it is bounding.
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
