/**
 * AI Assist — the editor half (ASSIST.md §5, DESIGN-ASSIST-PHASE-A §2.2):
 * `compositeCommand` (one undo entry for a whole accepted batch, inverted in
 * reverse) and the `Editor.previewOps` / `acceptOps` / `rejectOps` ghost-preview
 * flow. The ghost is a top overlay only — the proposal enters the document and
 * history solely on Accept, as a SINGLE composite command.
 *
 * Headless Konva, same trick as backdrop.test.ts: node-canvas backs
 * `Util.createCanvasElement` (import "konva/canvas-backend" first), and the few
 * DOM surfaces the editor touches are stubbed. Assertions focus on the document +
 * command/undo state (the runner is DOM-less), not pixels.
 */
import "konva/canvas-backend";
import { describe, expect, it, afterEach } from "vitest";
import { validateDocument } from "@justpaint/document";
import type { Document, LineStroke, Op } from "@justpaint/document";
import { Editor } from "../src/editor";
import { addLayerCommand, addStrokeCommand, compositeCommand } from "../src/history";
import type { Command } from "../src/history";

// --- headless stubs (plain node env; installed once, before any Editor) -------

class StubResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

const g = globalThis as { ResizeObserver?: unknown; window?: unknown };
g.ResizeObserver ??= StubResizeObserver;
g.window ??= { addEventListener: (): void => {}, removeEventListener: (): void => {} };

const VIEWPORT = { width: 800, height: 600 };

function fakeContainer(): HTMLDivElement {
  return {
    clientWidth: VIEWPORT.width,
    clientHeight: VIEWPORT.height,
    style: { cursor: "" },
    addEventListener: (): void => {},
    removeEventListener: (): void => {},
  } as unknown as HTMLDivElement;
}

// --- document + op helpers ----------------------------------------------------

function doc(): Document {
  return {
    version: 1,
    width: 100,
    height: 100,
    background: "#ffffff",
    layers: [{ id: "L1", name: "Layer 1", visible: true, opacity: 1, strokes: [] }],
  };
}

function lineStroke(id: string): LineStroke {
  return {
    id,
    type: "line",
    composite: "source-over",
    points: [
      [1, 1],
      [10, 10],
    ],
    stroke: "#000000",
    strokeWidth: 2,
  };
}

/** A deep, structural snapshot (the editor mutates its doc in place, by reference). */
function clone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T;
}

let ed: Editor | null = null;
afterEach(() => {
  ed?.destroy();
  ed = null;
});

function editor(d: Document): Editor {
  ed = new Editor(fakeContainer(), d);
  return ed;
}

// --- compositeCommand ---------------------------------------------------------

describe("compositeCommand", () => {
  it("apply then invert restores the exact pre-batch document (add_layer + add_stroke on it)", () => {
    const d = doc();
    const before = clone(d);

    const composite = compositeCommand(
      [
        addLayerCommand({ id: "NEW", name: "Roof", visible: true, opacity: 1, strokes: [] }, 1),
        addStrokeCommand("NEW", lineStroke("s1")),
      ],
      "AI assist",
    );

    composite.apply(d);
    expect(d.layers.map((l) => l.id)).toEqual(["L1", "NEW"]);
    expect(d.layers[1]!.strokes.map((s) => s.id)).toEqual(["s1"]);
    expect(() => validateDocument(d)).not.toThrow();

    composite.invert(d);
    expect(d).toEqual(before);
    expect(() => validateDocument(d)).not.toThrow();
  });

  it("invert runs children in REVERSE order", () => {
    const calls: string[] = [];
    const spy = (name: string): Command => ({
      label: name,
      apply: () => {
        calls.push(`apply:${name}`);
      },
      invert: () => {
        calls.push(`invert:${name}`);
      },
    });

    const composite = compositeCommand([spy("a"), spy("b"), spy("c")], "batch");
    composite.apply(doc());
    composite.invert(doc());

    expect(calls).toEqual([
      "apply:a",
      "apply:b",
      "apply:c",
      "invert:c",
      "invert:b",
      "invert:a",
    ]);
  });

  it("is one History entry: a single execute + undo round-trips the whole batch", () => {
    const d = doc();
    const before = clone(d);
    const e = editor(d);
    // Drive through the public flow so the History wiring is exercised end to end.
    e.previewOps([
      { kind: "add_layer", id: "b1", name: "Roof" },
      { kind: "add_stroke", layerId: "b1", stroke: lineStroke("s1") },
    ]);
    e.acceptOps();
    expect(e.canUndo()).toBe(true);

    e.undo();
    expect(e.canUndo()).toBe(false);
    expect(e.getDocument()).toEqual(before);
  });
});

// --- Editor ghost-preview flow -----------------------------------------------

describe("Editor previewOps / acceptOps / rejectOps", () => {
  const proposal: Op[] = [
    { kind: "add_layer", id: "b1", name: "Roof" },
    { kind: "add_stroke", layerId: "b1", stroke: lineStroke("s-roof") },
    { kind: "add_stroke", layerId: "L1", stroke: lineStroke("s-base") },
  ];

  it("previewOps leaves the document and history untouched (a proposal, not a commit)", () => {
    const e = editor(doc());
    const before = clone(e.getDocument());

    e.previewOps(proposal);

    expect(e.getDocument()).toEqual(before);
    expect(e.canUndo()).toBe(false);
  });

  it("rejectOps discards the proposal — document unchanged, nothing to undo", () => {
    const e = editor(doc());
    const before = clone(e.getDocument());

    e.previewOps(proposal);
    e.rejectOps();

    expect(e.getDocument()).toEqual(before);
    expect(e.canUndo()).toBe(false);
  });

  it("acceptOps commits the batch (layers + strokes present) as ONE undoable entry", () => {
    const e = editor(doc());
    e.previewOps(proposal);
    const beforeAccept = clone(e.getDocument());

    e.acceptOps();

    const layers = e.getDocument().layers;
    // add_layer added a fresh top layer named "Roof"; the batch id "b1" is not reused.
    expect(layers.map((l) => l.name)).toEqual(["Layer 1", "Roof"]);
    expect(layers.some((l) => l.id === "b1")).toBe(false);
    const base = layers.find((l) => l.name === "Layer 1")!;
    const roof = layers.find((l) => l.name === "Roof")!;
    // add_stroke resolved "L1" (existing) and "b1" (batch layer) correctly.
    expect(base.strokes.map((s) => s.id)).toEqual(["s-base"]);
    expect(roof.strokes.map((s) => s.id)).toEqual(["s-roof"]);
    expect(() => validateDocument(e.getDocument())).not.toThrow();

    expect(e.canUndo()).toBe(true);
    // A SINGLE undo removes the whole batch → back to the pre-accept document.
    e.undo();
    expect(e.getDocument()).toEqual(beforeAccept);
    expect(e.canUndo()).toBe(false);
  });

  it("threads a running top index so multiple add_layer ops don't collide in z-order", () => {
    const e = editor(doc());
    e.previewOps([
      { kind: "add_layer", id: "b1", name: "A" },
      { kind: "add_layer", id: "b2", name: "B" },
    ]);
    e.acceptOps();
    // B stacks on top of A (indices 1 then 2), not both clamped to the same slot.
    expect(e.getDocument().layers.map((l) => l.name)).toEqual(["Layer 1", "A", "B"]);
  });

  it("previewOps replaces a prior proposal rather than stacking", () => {
    const e = editor(doc());
    e.previewOps([{ kind: "add_stroke", layerId: "L1", stroke: lineStroke("first") }]);
    e.previewOps([{ kind: "add_stroke", layerId: "L1", stroke: lineStroke("second") }]);
    e.acceptOps();
    // Only the second proposal is committed.
    expect(e.getDocument().layers[0]!.strokes.map((s) => s.id)).toEqual(["second"]);
  });

  it("acceptOps with no pending proposal is a no-op", () => {
    const e = editor(doc());
    const before = clone(e.getDocument());
    e.acceptOps();
    expect(e.getDocument()).toEqual(before);
    expect(e.canUndo()).toBe(false);
  });

  it("an empty batch is a true no-op: no phantom undo entry", () => {
    const e = editor(doc());
    const before = clone(e.getDocument());

    e.previewOps([]);
    e.acceptOps();

    expect(e.getDocument()).toEqual(before);
    // Committing an empty composite would otherwise push a no-op onto the undo
    // stack (canUndo() flips true, next Ctrl+Z does nothing) — assert it doesn't.
    expect(e.canUndo()).toBe(false);
  });
});
