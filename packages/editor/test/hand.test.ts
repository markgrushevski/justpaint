/**
 * The hand (pan) tool + Editor.toDocumentCoords.
 *
 * The hand routes pointerdown into the SAME pan path as the middle-button
 * drag — before the stroke-gesture pipeline, without the insideDocument gate —
 * so it can never produce a stroke or touch the history. toDocumentCoords is
 * the host-facing inverse of the stage transform (client → logical), for a
 * cursor-coordinates readout.
 *
 * Headless like backdrop.test.ts: node-canvas backs Konva via
 * "konva/canvas-backend" (import BEFORE any stage exists); the editor's own DOM
 * surfaces (container, window, ResizeObserver) are stubbed below — the window
 * stub here additionally CAPTURES listeners so the tests can dispatch the
 * escape/pointerup fallbacks the editor registers.
 */
import "konva/canvas-backend";
import Konva from "konva";
import { afterEach, describe, expect, it } from "vitest";
import { Editor } from "../src/editor";
import { TOOLS } from "../src/tools/index";
import type { Document, Layer } from "@justpaint/document";

// --- headless stubs (plain node env; installed once, before any Editor) -----

class StubResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

type WinListener = (e: unknown) => void;
const winListeners = new Map<string, Set<WinListener>>();

const g = globalThis as { ResizeObserver?: unknown; window?: unknown };
g.ResizeObserver ??= StubResizeObserver;
g.window ??= {
  addEventListener: (type: string, fn: WinListener): void => {
    let set = winListeners.get(type);
    if (!set) {
      set = new Set();
      winListeners.set(type, set);
    }
    set.add(fn);
  },
  removeEventListener: (type: string, fn: WinListener): void => {
    winListeners.get(type)?.delete(fn);
  },
};

/** Dispatch a window-level fallback the editor registered (keydown, pointerup…). */
function fireWindow(type: string, e: unknown): void {
  for (const fn of winListeners.get(type) ?? []) fn(e);
}

const VIEWPORT = { width: 800, height: 600 };

/**
 * The minimal container surface the editor touches. `style.cursor` backs the
 * grab/grabbing assertions; `getBoundingClientRect` (with an optional offset)
 * backs toDocumentCoords' client→container translation.
 */
function fakeContainer(offset: { left?: number; top?: number } = {}): HTMLDivElement {
  const left = offset.left ?? 0;
  const top = offset.top ?? 0;
  return {
    clientWidth: VIEWPORT.width,
    clientHeight: VIEWPORT.height,
    style: { cursor: "" },
    addEventListener: (): void => {},
    removeEventListener: (): void => {},
    getBoundingClientRect: () => ({
      left,
      top,
      width: VIEWPORT.width,
      height: VIEWPORT.height,
      right: left + VIEWPORT.width,
      bottom: top + VIEWPORT.height,
      x: left,
      y: top,
      toJSON: (): unknown => ({}),
    }),
  } as unknown as HTMLDivElement;
}

// --- document helpers (mirroring backdrop.test.ts) ---------------------------

function layer(id: string): Layer {
  return { id, name: id, visible: true, opacity: 1, strokes: [] };
}

function doc(overrides: Partial<Document> = {}): Document {
  return {
    version: 1,
    width: 100,
    height: 100,
    background: "#ffffff",
    layers: [layer("L1")],
    ...overrides,
  };
}

// The 100×100 doc fit into 800×600: zoom 6, centered → panX 100, panY 0.
const FIT = { zoom: 6, panX: 100, panY: 0 };

// --- editor plumbing ----------------------------------------------------------

let ed: Editor | null = null;
afterEach(() => {
  ed?.destroy();
  ed = null;
  winListeners.clear();
});

function editor(d: Document, container: HTMLDivElement = fakeContainer()): Editor {
  ed = new Editor(container, d);
  return ed;
}

/** Private reach-ins, deliberate (the suite's established pattern). */
function stageOf(e: Editor): Konva.Stage {
  return (e as unknown as { stage: Konva.Stage }).stage;
}

function previewGroupOf(e: Editor): Konva.Group | null {
  return (e as unknown as { previewGroup: Konva.Group | null }).previewGroup;
}

function cursorRingOf(e: Editor): Konva.Circle | null {
  return (e as unknown as { cursorRing: Konva.Circle | null }).cursorRing;
}

function autoFitOf(e: Editor): boolean {
  return (e as unknown as { autoFit: boolean }).autoFit;
}

type PointerName = "pointerdown" | "pointermove" | "pointerup";

interface FireOpts {
  button?: number;
  pointerId?: number;
  pointerType?: "mouse" | "touch";
}

/**
 * Drive the editor's pointer pipeline headlessly at SCREEN (stage) coords:
 * register the pointer with Konva, then fire the stage event. For touch,
 * `button` is absent — mirroring the raw TouchEvents Konva can hand through.
 */
function fireScreen(stage: Konva.Stage, name: PointerName, sx: number, sy: number, opts: FireOpts = {}): void {
  const pointerType = opts.pointerType ?? "mouse";
  const native = {
    type: name,
    button: pointerType === "touch" ? undefined : (opts.button ?? 0),
    pointerId: opts.pointerId ?? 1,
    pointerType,
    pressure: 0.5,
    clientX: sx,
    clientY: sy,
    preventDefault: (): void => {},
  };
  stage.setPointersPositions(native);
  stage.fire(name, { evt: native });
}

/** Same, addressed in LOGICAL document coords through the fit view. */
function fireLogical(stage: Konva.Stage, name: PointerName, x: number, y: number): void {
  fireScreen(stage, name, x * FIT.zoom + FIT.panX, y * FIT.zoom + FIT.panY);
}

function strokeCounts(e: Editor): number[] {
  return e.getLayers().map((l) => l.strokeCount);
}

// --- hand (pan) tool ----------------------------------------------------------

describe("hand (pan) tool", () => {
  it("primary mouse drag pans the view — no stroke, history untouched, zoom unchanged", () => {
    const d = doc();
    const e = editor(d);
    e.setTool(TOOLS.hand);
    const stage = stageOf(e);
    expect(stage.position()).toEqual({ x: FIT.panX, y: FIT.panY });

    fireScreen(stage, "pointerdown", 400, 300); // logical (50,50) — inside the doc
    fireScreen(stage, "pointermove", 450, 320);

    // The view moved by the drag delta through the SAME transform path as
    // middle-button pan; a manual pan also turns auto-fit off (a resize must
    // not snap the view back).
    expect(stage.position()).toEqual({ x: FIT.panX + 50, y: FIT.panY + 20 });
    expect(e.getZoom()).toBe(FIT.zoom);
    expect(autoFitOf(e)).toBe(false);
    // No gesture ever started: no preview, and after release no stroke/history.
    expect(previewGroupOf(e)).toBeNull();

    fireScreen(stage, "pointerup", 450, 320);
    expect(strokeCounts(e)).toEqual([0]);
    expect(e.canUndo()).toBe(false);
    expect(e.canRedo()).toBe(false);
    expect(e.getDocument()).toBe(d); // the document object was never touched
  });

  it("grabs from the LETTERBOX too (no insideDocument gate) — where a stroke tool ignores the down", () => {
    const e = editor(doc());
    const stage = stageOf(e);
    // Screen (10,10) is logical (-15, 1.67): outside the 100×100 doc rect.
    e.setTool(TOOLS.hand);
    fireScreen(stage, "pointerdown", 10, 10);
    fireScreen(stage, "pointermove", 60, 10);
    expect(stage.position()).toEqual({ x: FIT.panX + 50, y: FIT.panY });
    fireScreen(stage, "pointerup", 60, 10);

    // Contrast: the pen refuses to START outside the document (existing gate).
    e.setTool(TOOLS.pen);
    fireScreen(stage, "pointerdown", 10, 10);
    expect(previewGroupOf(e)).toBeNull();
    fireScreen(stage, "pointermove", 60, 10);
    fireScreen(stage, "pointerup", 60, 10);
    expect(strokeCounts(e)).toEqual([0]);
    expect(e.canUndo()).toBe(false);
  });

  it("single-finger TOUCH drag pans while the hand is active (raw TouchEvent shape: no button)", () => {
    const e = editor(doc());
    e.setTool(TOOLS.hand);
    const stage = stageOf(e);

    fireScreen(stage, "pointerdown", 400, 300, { pointerType: "touch", pointerId: 7 });
    fireScreen(stage, "pointermove", 380, 280, { pointerType: "touch", pointerId: 7 });
    expect(stage.position()).toEqual({ x: FIT.panX - 20, y: FIT.panY - 20 });

    fireScreen(stage, "pointerup", 380, 280, { pointerType: "touch", pointerId: 7 });
    // The pan ended: further moves of that pointer no longer drag the view.
    fireScreen(stage, "pointermove", 300, 200, { pointerType: "touch", pointerId: 7 });
    expect(stage.position()).toEqual({ x: FIT.panX - 20, y: FIT.panY - 20 });
    expect(strokeCounts(e)).toEqual([0]);
    expect(e.canUndo()).toBe(false);
  });

  it("a second pointer mid-pan neither re-anchors nor hijacks the drag", () => {
    const e = editor(doc());
    e.setTool(TOOLS.hand);
    const stage = stageOf(e);

    fireScreen(stage, "pointerdown", 400, 300, { pointerType: "touch", pointerId: 7 });
    fireScreen(stage, "pointermove", 410, 300, { pointerType: "touch", pointerId: 7 });
    expect(stage.position()).toEqual({ x: FIT.panX + 10, y: FIT.panY });

    // Second finger down + move: ignored (beginPan no-ops while a pan is live).
    fireScreen(stage, "pointerdown", 0, 0, { pointerType: "touch", pointerId: 8 });
    fireScreen(stage, "pointermove", 100, 100, { pointerType: "touch", pointerId: 8 });
    expect(stage.position()).toEqual({ x: FIT.panX + 10, y: FIT.panY });

    // The original finger still owns the pan.
    fireScreen(stage, "pointermove", 420, 300, { pointerType: "touch", pointerId: 7 });
    expect(stage.position()).toEqual({ x: FIT.panX + 20, y: FIT.panY });
  });

  it("container cursor: grab while armed, grabbing while dragging, restored on tool switch", () => {
    const container = fakeContainer();
    const e = editor(doc(), container);
    const stage = stageOf(e);
    expect(container.style.cursor).toBe("");

    e.setTool(TOOLS.hand);
    expect(container.style.cursor).toBe("grab");

    fireScreen(stage, "pointerdown", 400, 300);
    expect(container.style.cursor).toBe("grabbing");
    fireScreen(stage, "pointerup", 400, 300);
    expect(container.style.cursor).toBe("grab");

    e.setTool(TOOLS.pen);
    expect(container.style.cursor).toBe("");

    // The always-available middle-button pan reports "grabbing" too…
    fireScreen(stage, "pointerdown", 400, 300, { button: 1 });
    expect(container.style.cursor).toBe("grabbing");
    fireScreen(stage, "pointerup", 400, 300, { button: 1 });
    // …and hands the cursor back afterwards (pen owns no grab cursor).
    expect(container.style.cursor).toBe("");
  });

  it("Escape abandons the pan cleanly (the pointercancel hardening, on a key)", () => {
    const container = fakeContainer();
    const e = editor(doc(), container);
    e.setTool(TOOLS.hand);
    const stage = stageOf(e);

    fireScreen(stage, "pointerdown", 400, 300);
    fireScreen(stage, "pointermove", 450, 300);
    expect(stage.position()).toEqual({ x: FIT.panX + 50, y: FIT.panY });
    expect(container.style.cursor).toBe("grabbing");

    fireWindow("keydown", { key: "Escape" });
    expect(container.style.cursor).toBe("grab"); // armed again, not dragging
    fireScreen(stage, "pointermove", 500, 300); // the dead pointer pans nothing
    expect(stage.position()).toEqual({ x: FIT.panX + 50, y: FIT.panY });
    expect(e.canUndo()).toBe(false);
  });

  it("a pointer released OUTSIDE the container ends the pan (window fallback)", () => {
    const container = fakeContainer();
    const e = editor(doc(), container);
    e.setTool(TOOLS.hand);
    const stage = stageOf(e);

    fireScreen(stage, "pointerdown", 400, 300);
    expect(container.style.cursor).toBe("grabbing");
    fireWindow("pointerup", {});
    expect(container.style.cursor).toBe("grab");
    fireScreen(stage, "pointermove", 500, 300);
    expect(stage.position()).toEqual({ x: FIT.panX, y: FIT.panY });
  });

  it("the brush-size cursor ring hides while the hand is active", () => {
    const e = editor(doc());
    e.setTool(TOOLS.pen);
    e.setCursorColor("#ff0000");
    const stage = stageOf(e);

    fireScreen(stage, "pointermove", 400, 300); // mouse hover → ring on
    expect(cursorRingOf(e)!.visible()).toBe(true);

    e.setTool(TOOLS.hand); // hand never draws — no brush-size ring
    expect(cursorRingOf(e)!.visible()).toBe(false);
    fireScreen(stage, "pointermove", 410, 300); // hovering with hand keeps it off
    expect(cursorRingOf(e)!.visible()).toBe(false);

    e.setTool(TOOLS.pen);
    expect(cursorRingOf(e)!.visible()).toBe(true);
  });

  it("switching to the hand MID-STROKE abandons the gesture instead of committing through it", () => {
    const e = editor(doc());
    e.setTool(TOOLS.pen);
    const stage = stageOf(e);

    fireLogical(stage, "pointerdown", 10, 10);
    fireLogical(stage, "pointermove", 20, 20);
    expect(previewGroupOf(e)).not.toBeNull(); // live stroke preview

    e.setTool(TOOLS.hand);
    expect(previewGroupOf(e)).toBeNull(); // dropped, not committed

    fireLogical(stage, "pointerup", 30, 30);
    expect(strokeCounts(e)).toEqual([0]);
    expect(e.canUndo()).toBe(false);
  });
});

// --- toDocumentCoords ----------------------------------------------------------

describe("Editor.toDocumentCoords", () => {
  it("round-trips the fit transform, client-relative to an OFFSET container, letterbox included", () => {
    // Container not at the page origin: client coords ≠ stage coords.
    const e = editor(doc(), fakeContainer({ left: 20, top: 40 }));

    // Viewport center = logical document center at the fit transform. (The
    // stage-transform inversion multiplies by 1/zoom — compare to precision.)
    const center = e.toDocumentCoords(20 + 400, 40 + 300);
    expect(center!.x).toBeCloseTo(50, 10);
    expect(center!.y).toBeCloseTo(50, 10);
    // The document's top-left corner sits at screen x = panX (100).
    const origin = e.toDocumentCoords(20 + 100, 40 + 0);
    expect(origin!.x).toBeCloseTo(0, 10);
    expect(origin!.y).toBeCloseTo(0, 10);

    // Letterbox: inside the container but outside the doc rect still maps —
    // RAW (unrounded), out-of-range values; clamping/rounding is the host's.
    const letterbox = e.toDocumentCoords(20 + 10, 40 + 10);
    expect(letterbox).not.toBeNull();
    expect(letterbox!.x).toBeCloseTo((10 - FIT.panX) / FIT.zoom, 10); // -15
    expect(letterbox!.y).toBeCloseTo(10 / FIT.zoom, 10); // 1.666…, no rounding
  });

  it("stays exact through a zoom and a hand pan (reads the LIVE stage transform)", () => {
    const e = editor(doc(), fakeContainer({ left: 20, top: 40 }));

    // Zoom ×1.2 anchored at the viewport center: the anchor's logical point is invariant.
    e.zoomBy(1.2, 400, 300); // view → zoom 7.2, pan (40, -60)
    const anchor = e.toDocumentCoords(20 + 400, 40 + 300);
    expect(anchor!.x).toBeCloseTo(50, 10);
    expect(anchor!.y).toBeCloseTo(50, 10);
    // Logical (10,10) → client (20 + 40 + 72, 40 - 60 + 72).
    const p = e.toDocumentCoords(132, 52);
    expect(p!.x).toBeCloseTo(10, 10);
    expect(p!.y).toBeCloseTo(10, 10);

    // A hand pan shifts the mapping with the view.
    e.setTool(TOOLS.hand);
    const stage = stageOf(e);
    fireScreen(stage, "pointerdown", 400, 300);
    fireScreen(stage, "pointermove", 450, 320); // pan → (90, -40)
    fireScreen(stage, "pointerup", 450, 320);
    const after = e.toDocumentCoords(20 + 90 + 72, 40 - 40 + 72);
    expect(after!.x).toBeCloseTo(10, 10);
    expect(after!.y).toBeCloseTo(10, 10);
  });

  it("returns null outside the stage container; edges are inclusive", () => {
    const e = editor(doc(), fakeContainer({ left: 20, top: 40 }));

    expect(e.toDocumentCoords(19, 40 + 50)).toBeNull(); // left of the container
    expect(e.toDocumentCoords(20 + 801, 40 + 50)).toBeNull(); // right of it
    expect(e.toDocumentCoords(20 + 400, 39)).toBeNull(); // above
    expect(e.toDocumentCoords(20 + 400, 40 + 601)).toBeNull(); // below

    expect(e.toDocumentCoords(20, 40)).not.toBeNull(); // top-left corner
    expect(e.toDocumentCoords(20 + 800, 40 + 600)).not.toBeNull(); // bottom-right corner
  });
});
