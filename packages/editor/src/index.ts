/**
 * `@justpaint/editor` — Konva + perfect-freehand vector editor for the v1
 * vector-document schema. Projects {@link Document}s to a Konva stage, captures
 * pointer drags into strokes via pure tools, and exports deterministic PNGs.
 */
export type * from "./types";
export { newId } from "./ids";
export { DEFAULT_STYLE } from "./style";
export { toKonva, stageConfig } from "./konva";
export { renderToPNG, renderToStage } from "./render";
export type { RenderOptions } from "./render";
export { Editor } from "./editor";
export type { CanvasBackdrop } from "./editor";
export { TOOLS } from "./tools/index";
export { penTool } from "./tools/pen";
export { eraserTool } from "./tools/eraser";
export { lineTool } from "./tools/line";
export { rectTool } from "./tools/rect";
export { ellipseTool } from "./tools/ellipse";
export { triangleTool } from "./tools/triangle";
export { handTool } from "./tools/hand";
