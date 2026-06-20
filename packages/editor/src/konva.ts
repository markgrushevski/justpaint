/**
 * Project a vector {@link Document} onto a `Konva.Stage` — the editor/preview
 * surface of the one shared renderer (DOCUMENT-FORMAT §6, §10).
 *
 * This is a PROJECTION, never Konva's `Stage.toJSON()` serialization. The
 * document is canonical; Konva nodes are derived. Per-layer isolation is
 * structural: one `Konva.Layer` (its own `<canvas>`) per document layer, plus
 * an isolated bottom layer for a non-null background so a full-canvas erase
 * (`destination-out`) can never punch through it.
 */
import Konva from "konva";
import { toFreehandOptions } from "@justpaint/document";
import type {
  Composite,
  Document,
  EllipseStroke,
  FreehandStroke,
  Layer,
  LineStroke,
  PolygonStroke,
  RectStroke,
  Stroke,
} from "@justpaint/document";
import { getStroke } from "perfect-freehand";

/** Konva's blend attr is the document `composite` value verbatim. */
type Composition = GlobalCompositeOperation;

/** Flatten perfect-freehand's `[[x,y],...]` outline to `[x,y,x,y,...]`. */
function flattenOutline(outline: readonly number[][]): number[] {
  const flat: number[] = [];
  for (const pt of outline) {
    const x = pt[0];
    const y = pt[1];
    if (x === undefined || y === undefined) continue;
    flat.push(x, y);
  }
  return flat;
}

/** Flatten a list of 2-tuple points to `[x,y,x,y,...]`. */
function flattenPoints(points: ReadonlyArray<readonly [number, number]>): number[] {
  const flat: number[] = [];
  for (const [x, y] of points) flat.push(x, y);
  return flat;
}

function toComposite(composite: Composite): Composition {
  return composite as Composition;
}

function freehandNode(s: FreehandStroke): Konva.Line {
  // Store-input-not-output: run getStroke at render time over the raw samples.
  const outline = getStroke(
    s.points.map((p) => [p[0], p[1], p[2]]),
    toFreehandOptions(s.brush),
  );
  return new Konva.Line({
    id: s.id,
    points: flattenOutline(outline),
    closed: true,
    fill: s.color,
    strokeWidth: 0,
    globalCompositeOperation: toComposite(s.composite),
  });
}

function lineNode(s: LineStroke): Konva.Line {
  return new Konva.Line({
    id: s.id,
    points: flattenPoints(s.points),
    stroke: s.stroke,
    strokeWidth: s.strokeWidth,
    lineCap: s.cap ?? "round",
    lineJoin: s.join ?? "round",
    tension: 0,
    globalCompositeOperation: toComposite(s.composite),
  });
}

function rectNode(s: RectStroke): Konva.Rect {
  const node = new Konva.Rect({
    id: s.id,
    x: s.x,
    y: s.y,
    width: s.width,
    height: s.height,
    globalCompositeOperation: toComposite(s.composite),
  });
  if (s.cornerRadius !== undefined) node.cornerRadius(s.cornerRadius);
  if (s.fill != null) node.fill(s.fill);
  if (s.stroke != null) {
    node.stroke(s.stroke);
    if (s.strokeWidth !== undefined) node.strokeWidth(s.strokeWidth);
  }
  return node;
}

function ellipseNode(s: EllipseStroke): Konva.Ellipse {
  const node = new Konva.Ellipse({
    id: s.id,
    x: s.cx,
    y: s.cy,
    radiusX: s.rx,
    radiusY: s.ry,
    globalCompositeOperation: toComposite(s.composite),
  });
  if (s.fill != null) node.fill(s.fill);
  if (s.stroke != null) {
    node.stroke(s.stroke);
    if (s.strokeWidth !== undefined) node.strokeWidth(s.strokeWidth);
  }
  return node;
}

function polygonNode(s: PolygonStroke): Konva.Line {
  const node = new Konva.Line({
    id: s.id,
    points: flattenPoints(s.points),
    closed: true,
    lineJoin: s.join ?? "round",
    globalCompositeOperation: toComposite(s.composite),
  });
  if (s.fill != null) node.fill(s.fill);
  if (s.stroke != null) {
    node.stroke(s.stroke);
    if (s.strokeWidth !== undefined) node.strokeWidth(s.strokeWidth);
  }
  return node;
}

/** Map a single document stroke to its Konva shape (DOCUMENT-FORMAT §6). */
function toNode(stroke: Stroke): Konva.Shape {
  switch (stroke.type) {
    case "freehand":
      return freehandNode(stroke);
    case "line":
      return lineNode(stroke);
    case "rect":
      return rectNode(stroke);
    case "ellipse":
      return ellipseNode(stroke);
    case "polygon":
      return polygonNode(stroke);
  }
}

/** Build the isolated bottom-most background layer (DOCUMENT-FORMAT §10 step 2). */
function backgroundLayer(color: string, width: number, height: number): Konva.Layer {
  const layer = new Konva.Layer({ listening: false });
  layer.add(
    new Konva.Rect({ x: 0, y: 0, width, height, fill: color }),
  );
  return layer;
}

/** Map a document layer to a `Konva.Layer`, applying `opacity`/`visible`. */
function toLayer(layer: Layer): Konva.Layer {
  const kLayer = new Konva.Layer({
    opacity: layer.opacity,
    visible: layer.visible,
  });
  for (const stroke of layer.strokes) kLayer.add(toNode(stroke));
  return kLayer;
}

/**
 * Project `doc` onto a fresh `Konva.Stage`. Pass a `container` to mount it in
 * the DOM (editor); omit it for a detached stage (the render worker sets its
 * own container/transform).
 */
export function toKonva(doc: Document, container?: HTMLDivElement): Konva.Stage {
  const stage = new Konva.Stage({
    container: container ?? document.createElement("div"),
    width: doc.width,
    height: doc.height,
  });

  if (doc.background != null) {
    stage.add(backgroundLayer(doc.background, doc.width, doc.height));
  }
  for (const layer of doc.layers) stage.add(toLayer(layer));

  return stage;
}
