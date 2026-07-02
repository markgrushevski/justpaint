/**
 * Command-based undo/redo over the canonical {@link Document} — the replacement
 * for the old raster app's PNG-snapshot history (ROADMAP Phase 2 / cross-cutting
 * cleanups). History is **runtime-only**: commands mutate the in-memory document
 * and are never persisted (the jsonb payload is always the current document, no
 * frames).
 *
 * A {@link Command} is a reversible mutation. `apply` performs (or re-performs,
 * on redo) it; `invert` undoes it. Each command captures whatever it needs to
 * invert **at construction time** (the removed layer, the old name, the source
 * index), so `apply`/`invert` are deterministic no matter when they run. Commands
 * that snapshot prior state take the live `doc` as their first constructor
 * argument and must be built immediately before being executed.
 *
 * Commands are keyed by stroke/layer `id`, never by array position, so they stay
 * correct as the document is edited around them (DOCUMENT-FORMAT §8).
 */
import type { Document, Layer, Stroke } from "@justpaint/document";

/** A reversible mutation of a {@link Document}. */
export interface Command {
  /** Short human label (for debugging / a future history panel). */
  readonly label: string;
  /** Perform the mutation (also used for redo). */
  apply(doc: Document): void;
  /** Reverse the mutation. */
  invert(doc: Document): void;
}

function layerById(doc: Document, layerId: string): Layer | undefined {
  return doc.layers.find((l) => l.id === layerId);
}

/** Move a layer (found by id) to `targetIndex`, clamped into range. */
function moveLayerTo(doc: Document, layerId: string, targetIndex: number): void {
  const from = doc.layers.findIndex((l) => l.id === layerId);
  if (from === -1) return;
  const [layer] = doc.layers.splice(from, 1);
  if (!layer) return;
  const clamped = Math.max(0, Math.min(targetIndex, doc.layers.length));
  doc.layers.splice(clamped, 0, layer);
}

/** Append `stroke` to a layer; undo removes it by id. */
export function addStrokeCommand(layerId: string, stroke: Stroke): Command {
  return {
    label: "draw",
    apply(doc) {
      layerById(doc, layerId)?.strokes.push(stroke);
    },
    invert(doc) {
      const layer = layerById(doc, layerId);
      if (!layer) return;
      const i = layer.strokes.findIndex((s) => s.id === stroke.id);
      if (i !== -1) layer.strokes.splice(i, 1);
    },
  };
}

/** Insert `layer` at `index`; undo removes it by id. */
export function addLayerCommand(layer: Layer, index: number): Command {
  return {
    label: "add layer",
    apply(doc) {
      const at = Math.max(0, Math.min(index, doc.layers.length));
      doc.layers.splice(at, 0, layer);
    },
    invert(doc) {
      const i = doc.layers.findIndex((l) => l.id === layer.id);
      if (i !== -1) doc.layers.splice(i, 1);
    },
  };
}

/** Remove a layer (snapshotting it + its index); undo restores it in place. */
export function removeLayerCommand(doc: Document, layerId: string): Command {
  const index = doc.layers.findIndex((l) => l.id === layerId);
  const snapshot = doc.layers[index];
  return {
    label: "remove layer",
    apply(d) {
      const i = d.layers.findIndex((l) => l.id === layerId);
      if (i !== -1) d.layers.splice(i, 1);
    },
    invert(d) {
      if (snapshot) d.layers.splice(Math.min(index, d.layers.length), 0, snapshot);
    },
  };
}

/** Reorder a layer to `toIndex`; undo returns it to its original index. */
export function moveLayerCommand(doc: Document, layerId: string, toIndex: number): Command {
  const fromIndex = doc.layers.findIndex((l) => l.id === layerId);
  return {
    label: "reorder layer",
    apply(d) {
      moveLayerTo(d, layerId, toIndex);
    },
    invert(d) {
      moveLayerTo(d, layerId, fromIndex);
    },
  };
}

/** Rename a layer (snapshotting the old name); undo restores it. */
export function renameLayerCommand(doc: Document, layerId: string, name: string): Command {
  const previous = layerById(doc, layerId)?.name ?? name;
  return {
    label: "rename layer",
    apply(d) {
      const layer = layerById(d, layerId);
      if (layer) layer.name = name;
    },
    invert(d) {
      const layer = layerById(d, layerId);
      if (layer) layer.name = previous;
    },
  };
}

/** Set a layer's `visible` flag (snapshotting the old value); undo restores it. */
export function setLayerVisibleCommand(doc: Document, layerId: string, visible: boolean): Command {
  const previous = layerById(doc, layerId)?.visible ?? visible;
  return {
    label: visible ? "show layer" : "hide layer",
    apply(d) {
      const layer = layerById(d, layerId);
      if (layer) layer.visible = visible;
    },
    invert(d) {
      const layer = layerById(d, layerId);
      if (layer) layer.visible = previous;
    },
  };
}

/** Set a layer's `opacity` (snapshotting the old value); undo restores it. */
export function setLayerOpacityCommand(doc: Document, layerId: string, opacity: number): Command {
  const previous = layerById(doc, layerId)?.opacity ?? opacity;
  return {
    label: "layer opacity",
    apply(d) {
      const layer = layerById(d, layerId);
      if (layer) layer.opacity = opacity;
    },
    invert(d) {
      const layer = layerById(d, layerId);
      if (layer) layer.opacity = previous;
    },
  };
}

/**
 * A bounded undo/redo stack. Operates on a {@link Document} passed to each call —
 * it never holds the document itself, so replacing the editor's document (a fresh
 * `loadDocument`) is handled by the caller clearing the stack.
 */
export class History {
  private readonly undoStack: Command[] = [];
  private readonly redoStack: Command[] = [];

  /** The most recent command's undo point, for a future history label. */
  get canUndo(): boolean {
    return this.undoStack.length > 0;
  }

  get canRedo(): boolean {
    return this.redoStack.length > 0;
  }

  /** Apply a command and record it; clears the redo stack (a new branch). */
  execute(doc: Document, cmd: Command): void {
    cmd.apply(doc);
    this.undoStack.push(cmd);
    this.redoStack.length = 0;
  }

  /** Undo the last command. Returns false when there's nothing to undo. */
  undo(doc: Document): boolean {
    const cmd = this.undoStack.pop();
    if (!cmd) return false;
    cmd.invert(doc);
    this.redoStack.push(cmd);
    return true;
  }

  /** Redo the last undone command. Returns false when there's nothing to redo. */
  redo(doc: Document): boolean {
    const cmd = this.redoStack.pop();
    if (!cmd) return false;
    cmd.apply(doc);
    this.undoStack.push(cmd);
    return true;
  }

  /** Drop all history (e.g. after loading a different document). */
  clear(): void {
    this.undoStack.length = 0;
    this.redoStack.length = 0;
  }
}
