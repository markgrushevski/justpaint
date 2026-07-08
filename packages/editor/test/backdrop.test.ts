/**
 * Editor.setCanvasBackdrop — the VIEW-ONLY backdrop layer (theme "paper",
 * transparency checkerboard). Presentation state only: painted below
 * everything, never part of the document, remounted across rerenders, and
 * structurally unable to leak into exports (renderToPNG builds a fresh stage
 * from the document — see src/render.ts).
 *
 * The Editor is browser-first, but Konva runs headless once node-canvas backs
 * `Util.createCanvasElement` (the same trick as @justpaint/render): import
 * "konva/canvas-backend" BEFORE any stage exists. Konva's own DOM paths are all
 * guarded on `Konva.isBrowser`; the few DOM surfaces the editor itself touches
 * (container, window, ResizeObserver) are stubbed below.
 */
import "konva/canvas-backend";
import Konva from "konva";
import { afterEach, describe, expect, it } from "vitest";
import { BRUSH_DEFAULTS } from "@justpaint/document";
import type { Document, FreehandStroke, Layer } from "@justpaint/document";
import { Editor } from "../src/editor";
import { fitView, type ViewState } from "../src/view";

// --- headless stubs (plain node env; installed once, before any Editor) -----

class StubResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

const g = globalThis as { ResizeObserver?: unknown; window?: unknown };
g.ResizeObserver ??= StubResizeObserver;
g.window ??= { addEventListener: (): void => {}, removeEventListener: (): void => {} };

const VIEWPORT = { width: 800, height: 600 };

/** The minimal container surface the editor touches (Konva skips the DOM headless). */
function fakeContainer(): HTMLDivElement {
  return {
    clientWidth: VIEWPORT.width,
    clientHeight: VIEWPORT.height,
    style: { cursor: "" }, // the editor reflects pan/hand state on style.cursor
    addEventListener: (): void => {},
    removeEventListener: (): void => {},
  } as unknown as HTMLDivElement;
}

// --- document helpers (mirroring history.test.ts) ----------------------------

function layer(id: string, strokes: FreehandStroke[] = []): Layer {
  return { id, name: id, visible: true, opacity: 1, strokes };
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

function stroke(id: string): FreehandStroke {
  return {
    id,
    type: "freehand",
    composite: "source-over",
    color: "#000000",
    points: [[1, 2, 0.5]],
    brush: BRUSH_DEFAULTS,
  };
}

// --- editor plumbing ---------------------------------------------------------

let ed: Editor | null = null;
afterEach(() => {
  ed?.destroy();
  ed = null;
});

function editor(d: Document): Editor {
  ed = new Editor(fakeContainer(), d);
  return ed;
}

/**
 * The stage / preview group / active-layer mapping are private, but their layer
 * ORDER is exactly what these tests guard — reach in deliberately.
 */
function stageOf(e: Editor): Konva.Stage {
  return (e as unknown as { stage: Konva.Stage }).stage;
}

function previewGroupOf(e: Editor): Konva.Group | null {
  return (e as unknown as { previewGroup: Konva.Group | null }).previewGroup;
}

function activeKonvaLayerOf(e: Editor): Konva.Layer | null {
  return (e as unknown as { activeKonvaLayer(): Konva.Layer | null }).activeKonvaLayer();
}

/** The single Rect a backdrop/background layer carries. */
function rectOf(l: Konva.Layer): Konva.Rect {
  return l.getChildren()[0] as Konva.Rect;
}

type PointerName = "pointerdown" | "pointermove" | "pointerup";

/**
 * Drive the editor's pointer pipeline headlessly: register the pointer with
 * Konva (so `getRelativePointerPosition` works), then fire the stage event the
 * editor listens for. `x`/`y` are LOGICAL document coords; the fit view maps
 * them to the screen coords Konva expects.
 */
function firePointer(stage: Konva.Stage, name: PointerName, x: number, y: number, view: ViewState): void {
  const native = {
    type: name,
    button: 0,
    pointerId: 1,
    pointerType: "mouse",
    pressure: 0.5,
    clientX: x * view.zoom + view.panX,
    clientY: y * view.zoom + view.panY,
    preventDefault: (): void => {},
  };
  stage.setPointersPositions(native);
  stage.fire(name, { evt: native });
}

// --- tests -------------------------------------------------------------------

describe("Editor.setCanvasBackdrop", () => {
  it("color: adds a non-listening bottom layer below background + doc layers, rect sized to the doc", () => {
    const d = doc();
    const e = editor(d);
    expect(stageOf(e).getLayers()).toHaveLength(2); // background + 1 doc layer

    e.setCanvasBackdrop({ type: "color", color: "#123456" });

    const layers = stageOf(e).getLayers();
    expect(layers).toHaveLength(3);
    const backdrop = layers[0]!;
    expect(backdrop.listening()).toBe(false);
    const r = rectOf(backdrop);
    expect(r.fill()).toBe("#123456");
    expect([r.x(), r.y(), r.width(), r.height()]).toEqual([0, 0, 100, 100]);
    // NOT clipped (unlike projected doc layers): the drop shadow and the outer
    // half of the border render OUTSIDE the doc rect — a clip would eat them.
    expect(backdrop.clipWidth()).toBeUndefined();
    expect(backdrop.clipHeight()).toBeUndefined();
    // "Paper on a desk": drop shadow + hairline border on the rect.
    expect(r.shadowColor()).toBe("black");
    expect(r.shadowOpacity()).toBe(0.22);
    expect(r.shadowForStrokeEnabled()).toBe(false);
    expect(r.stroke()).toBe("rgb(0 0 0 / 25%)");
    expect(r.strokeWidth()).toBe(1);
    expect(r.strokeScaleEnabled()).toBe(false); // 1 SCREEN px at any zoom
    // The document background (white) sits ABOVE the backdrop.
    expect(rectOf(layers[1]!).fill()).toBe("#ffffff");
    // Presentation state only: the document is untouched.
    expect(e.getDocument()).toBe(d);
  });

  it("pattern: sets the tile image + repeat, and counter-scales it against the zoom", () => {
    const e = editor(doc());
    const tile = Konva.Util.createCanvasElement(); // node-canvas backed
    e.setCanvasBackdrop({ type: "pattern", image: tile });

    const r = rectOf(stageOf(e).getLayers()[0]!);
    expect(r.fillPatternImage()).toBe(tile);
    expect(r.fillPatternRepeat()).toBe("repeat");
    // Tiles are ~screen-space: pattern scale cancels the current fit zoom.
    const z0 = e.getZoom();
    expect(z0).toBeGreaterThan(1); // 100² doc fit into 800×600 zooms in
    expect(r.fillPatternScaleX()).toBeCloseTo(1 / z0);
    expect(r.fillPatternScaleY()).toBeCloseTo(1 / z0);

    e.zoomIn();
    const z1 = e.getZoom();
    expect(z1).toBeGreaterThan(z0);
    expect(r.fillPatternScaleX()).toBeCloseTo(1 / z1);
    expect(r.fillPatternScaleY()).toBeCloseTo(1 / z1);

    // The paper chrome applies to the pattern flavor too.
    expect(r.shadowColor()).toBe("black");
    expect(r.shadowBlur()).toBeCloseTo(12 / z1);
    expect(r.stroke()).toBe("rgb(0 0 0 / 25%)");
  });

  it("shadow: ~screen-space — blur/offset counter-scale against the zoom like the pattern tiles", () => {
    const e = editor(doc());
    e.setCanvasBackdrop({ type: "color", color: "#123456" });
    const r = rectOf(stageOf(e).getLayers()[0]!);

    const z0 = e.getZoom();
    expect(z0).toBeGreaterThan(1); // 100² doc fit into 800×600 zooms in
    expect(r.shadowBlur()).toBeCloseTo(12 / z0);
    expect(r.shadowOffsetX()).toBe(0);
    expect(r.shadowOffsetY()).toBeCloseTo(2 / z0);

    e.zoomIn();
    const z1 = e.getZoom();
    expect(z1).toBeGreaterThan(z0);
    expect(r.shadowBlur()).toBeCloseTo(12 / z1);
    expect(r.shadowOffsetX()).toBe(0);
    expect(r.shadowOffsetY()).toBeCloseTo(2 / z1);
  });

  it("survives loadDocument, remounted at the bottom and sized to the NEW doc", () => {
    const e = editor(doc());
    e.setCanvasBackdrop({ type: "color", color: "#123456" });

    e.loadDocument(doc({ width: 200, height: 50, layers: [layer("A"), layer("B")] }));

    const layers = stageOf(e).getLayers(); // the stage was rebuilt — re-read it
    expect(layers).toHaveLength(4); // backdrop + background + 2 doc layers
    const r = rectOf(layers[0]!);
    expect(r.fill()).toBe("#123456");
    expect([r.width(), r.height()]).toEqual([200, 50]);
    expect(layers[0]!.clipWidth()).toBeUndefined(); // still unclipped (shadow/border)
    expect(r.shadowColor()).toBe("black"); // paper chrome survives the remount
  });

  it("null clears the backdrop, including across a later rerender", () => {
    const e = editor(doc());
    e.setCanvasBackdrop({ type: "color", color: "#123456" });
    e.setCanvasBackdrop(null);

    expect(stageOf(e).getLayers()).toHaveLength(2);
    expect(rectOf(stageOf(e).getLayers()[0]!).fill()).toBe("#ffffff"); // background is bottom again

    e.loadDocument(doc()); // rebuilds the stage — must NOT resurrect the backdrop
    expect(stageOf(e).getLayers()).toHaveLength(2);
  });

  it("a drawn stroke still targets the ACTIVE doc layer with backdrop + background present", () => {
    const e = editor(
      doc({ layers: [layer("under", [stroke("s-under")]), layer("top", [stroke("s-top")])] }),
    );
    e.setCanvasBackdrop({ type: "color", color: "#0f0f0f" });
    e.setActiveLayer("under");

    // Screen coords for logical points, mirroring the editor's own auto-fit.
    const view = fitView(100, 100, VIEWPORT.width, VIEWPORT.height);
    const stage = stageOf(e);
    firePointer(stage, "pointerdown", 10, 10, view);
    firePointer(stage, "pointermove", 20, 20, view);

    // Mid-gesture the live preview must mount on the ACTIVE layer's stage
    // layer: [0 backdrop] [1 background] [2 "under"] [3 "top"].
    const preview = previewGroupOf(e);
    expect(preview).not.toBeNull();
    expect(preview!.getLayer()).toBe(stage.getLayers()[2]);

    firePointer(stage, "pointerup", 30, 30, view);

    // The stroke committed to the active DOCUMENT layer, not a neighbour.
    const docLayers = e.getDocument().layers;
    expect(docLayers[0]!.strokes).toHaveLength(2);
    expect(docLayers[1]!.strokes).toHaveLength(1);

    // The commit rerendered onto a FRESH stage: backdrop still bottom-most, and
    // doc layer i still projects to stage index i + 2.
    const layers = stageOf(e).getLayers();
    expect(layers).toHaveLength(4);
    expect(rectOf(layers[0]!).fill()).toBe("#0f0f0f");
    expect(layers[2]!.findOne("#s-under")).toBeDefined();
    expect(layers[3]!.findOne("#s-top")).toBeDefined();
  });

  it("addLayer keeps the active-layer mapping correct with a backdrop (index-math regression)", () => {
    const e = editor(doc({ layers: [layer("base", [stroke("s-base")])] }));
    e.setCanvasBackdrop({ type: "color", color: "#0f0f0f" });

    // Active "base" → stage index 2 (backdrop, background, base).
    expect(activeKonvaLayerOf(e)).toBe(stageOf(e).getLayers()[2]);
    expect(activeKonvaLayerOf(e)!.findOne("#s-base")).toBeDefined();

    // A new top layer becomes active → stage index 3; "base" keeps its slot.
    e.addLayer("L2");
    expect(stageOf(e).getLayers()).toHaveLength(4);
    expect(activeKonvaLayerOf(e)).toBe(stageOf(e).getLayers()[3]);
    expect(activeKonvaLayerOf(e)!.getChildren()).toHaveLength(0);
    expect(stageOf(e).getLayers()[2]!.findOne("#s-base")).toBeDefined();

    // Background OFF: the offset must drop to 1 (backdrop only).
    e.loadDocument(doc({ background: null, layers: [layer("solo", [stroke("s-solo")])] }));
    expect(stageOf(e).getLayers()).toHaveLength(2);
    expect(activeKonvaLayerOf(e)).toBe(stageOf(e).getLayers()[1]);
    expect(activeKonvaLayerOf(e)!.findOne("#s-solo")).toBeDefined();
  });
});
