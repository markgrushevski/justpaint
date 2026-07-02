import { validateDocument } from "@justpaint/document";
import type { Document, FreehandStroke, Layer } from "@justpaint/document";
import { describe, expect, it } from "vitest";
import {
  addLayerCommand,
  addStrokeCommand,
  History,
  moveLayerCommand,
  removeLayerCommand,
  renameLayerCommand,
  setLayerOpacityCommand,
  setLayerVisibleCommand,
} from "../src/history";

function layer(id: string, name = id): Layer {
  return { id, name, visible: true, opacity: 1, strokes: [] };
}

function doc(...layers: Layer[]): Document {
  return {
    version: 1,
    width: 100,
    height: 100,
    background: "#ffffff",
    layers: layers.length ? layers : [layer("L1")],
  };
}

function stroke(id: string): FreehandStroke {
  return {
    id,
    type: "freehand",
    composite: "source-over",
    color: "#000000",
    points: [[1, 2, 0.5]],
    brush: {
      size: 16,
      thinning: 0.5,
      smoothing: 0.5,
      streamline: 0.5,
      simulatePressure: true,
      taperStart: 0,
      taperEnd: 0,
    },
  };
}

/** Layer ids in document (z) order. */
function order(d: Document): string[] {
  return d.layers.map((l) => l.id);
}

describe("stroke commands", () => {
  it("addStroke apply pushes, invert removes by id, and the doc stays valid", () => {
    const d = doc(layer("L1"));
    const cmd = addStrokeCommand("L1", stroke("s1"));

    cmd.apply(d);
    expect(d.layers[0]!.strokes.map((s) => s.id)).toEqual(["s1"]);
    expect(() => validateDocument(d)).not.toThrow();

    cmd.invert(d);
    expect(d.layers[0]!.strokes).toEqual([]);
    expect(() => validateDocument(d)).not.toThrow();
  });

  it("invert removes only the matching stroke, not its neighbours", () => {
    const d = doc(layer("L1"));
    addStrokeCommand("L1", stroke("a")).apply(d);
    const cmd = addStrokeCommand("L1", stroke("b"));
    cmd.apply(d);
    addStrokeCommand("L1", stroke("c")).apply(d);

    cmd.invert(d);
    expect(d.layers[0]!.strokes.map((s) => s.id)).toEqual(["a", "c"]);
  });
});

describe("layer commands", () => {
  it("addLayer inserts at an index and undo removes it", () => {
    const d = doc(layer("A"), layer("B"));
    const cmd = addLayerCommand(layer("C"), 1);

    cmd.apply(d);
    expect(order(d)).toEqual(["A", "C", "B"]);
    expect(() => validateDocument(d)).not.toThrow();

    cmd.invert(d);
    expect(order(d)).toEqual(["A", "B"]);
  });

  it("removeLayer snapshots the layer + index; undo restores it in place", () => {
    const d = doc(layer("A"), layer("B"), layer("C"));
    d.layers[1]!.name = "middle";
    const cmd = removeLayerCommand(d, "B");

    cmd.apply(d);
    expect(order(d)).toEqual(["A", "C"]);

    cmd.invert(d);
    expect(order(d)).toEqual(["A", "B", "C"]);
    expect(d.layers[1]!.name).toBe("middle"); // restored the exact layer
  });

  it("moveLayer reorders and undo returns to the original index", () => {
    const d = doc(layer("A"), layer("B"), layer("C"));
    const cmd = moveLayerCommand(d, "A", 2);

    cmd.apply(d);
    expect(order(d)).toEqual(["B", "C", "A"]);

    cmd.invert(d);
    expect(order(d)).toEqual(["A", "B", "C"]);
  });

  it("rename / visible / opacity snapshot the old value and restore it", () => {
    const d = doc(layer("A"));

    const rename = renameLayerCommand(d, "A", "renamed");
    rename.apply(d);
    expect(d.layers[0]!.name).toBe("renamed");
    rename.invert(d);
    expect(d.layers[0]!.name).toBe("A");

    const hide = setLayerVisibleCommand(d, "A", false);
    hide.apply(d);
    expect(d.layers[0]!.visible).toBe(false);
    hide.invert(d);
    expect(d.layers[0]!.visible).toBe(true);

    const fade = setLayerOpacityCommand(d, "A", 0.25);
    fade.apply(d);
    expect(d.layers[0]!.opacity).toBe(0.25);
    fade.invert(d);
    expect(d.layers[0]!.opacity).toBe(1);
  });
});

describe("History", () => {
  it("execute/undo/redo walk the stack and toggle canUndo/canRedo", () => {
    const d = doc(layer("L1"));
    const h = new History();
    expect(h.canUndo).toBe(false);
    expect(h.canRedo).toBe(false);

    h.execute(d, addStrokeCommand("L1", stroke("s1")));
    h.execute(d, addStrokeCommand("L1", stroke("s2")));
    expect(d.layers[0]!.strokes.map((s) => s.id)).toEqual(["s1", "s2"]);
    expect(h.canUndo).toBe(true);
    expect(h.canRedo).toBe(false);

    expect(h.undo(d)).toBe(true);
    expect(d.layers[0]!.strokes.map((s) => s.id)).toEqual(["s1"]);
    expect(h.canRedo).toBe(true);

    expect(h.redo(d)).toBe(true);
    expect(d.layers[0]!.strokes.map((s) => s.id)).toEqual(["s1", "s2"]);
  });

  it("a new execute clears the redo stack (a new branch)", () => {
    const d = doc(layer("L1"));
    const h = new History();
    h.execute(d, addStrokeCommand("L1", stroke("s1")));
    h.undo(d);
    expect(h.canRedo).toBe(true);

    h.execute(d, addStrokeCommand("L1", stroke("s2")));
    expect(h.canRedo).toBe(false);
    expect(d.layers[0]!.strokes.map((s) => s.id)).toEqual(["s2"]);
  });

  it("undo/redo return false at the ends and leave the document untouched", () => {
    const d = doc(layer("L1"));
    const h = new History();
    expect(h.undo(d)).toBe(false);
    expect(h.redo(d)).toBe(false);
    expect(order(d)).toEqual(["L1"]);
  });

  it("clear() drops both stacks", () => {
    const d = doc(layer("L1"));
    const h = new History();
    h.execute(d, addStrokeCommand("L1", stroke("s1")));
    h.undo(d);
    h.clear();
    expect(h.canUndo).toBe(false);
    expect(h.canRedo).toBe(false);
  });

  it("a full undo of a mixed sequence returns to the start, staying valid throughout", () => {
    const d = doc(layer("A"));
    const h = new History();
    const before = order(d);

    h.execute(d, addStrokeCommand("A", stroke("s1")));
    h.execute(d, addLayerCommand(layer("B"), 1));
    h.execute(d, moveLayerCommand(d, "A", 1));
    h.execute(d, setLayerOpacityCommand(d, "B", 0.5));
    expect(() => validateDocument(d)).not.toThrow();

    while (h.canUndo) {
      h.undo(d);
      expect(() => validateDocument(d)).not.toThrow();
    }
    expect(order(d)).toEqual(before);
    expect(d.layers[0]!.strokes).toEqual([]);
  });
});
