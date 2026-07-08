import type { Tool, ToolId } from "../types";
import { penTool } from "./pen";
import { eraserTool } from "./eraser";
import { lineTool } from "./line";
import { rectTool } from "./rect";
import { ellipseTool } from "./ellipse";
import { triangleTool } from "./triangle";
import { handTool } from "./hand";

/**
 * The tool registry, keyed by {@link ToolId}.
 *
 * Most entries are pure stroke tools implementing `buildStroke` (no Konva, no
 * mutable state); `hand` is the pan-only {@link PanTool} (kind: "pan", no
 * `buildStroke`). The editor looks tools up here by id and routes on `kind`.
 * Insertion order is the host toolbar's display order — hand sits last, after
 * the drawing tools.
 */
export const TOOLS: Record<ToolId, Tool> = {
  pen: penTool,
  eraser: eraserTool,
  line: lineTool,
  rect: rectTool,
  ellipse: ellipseTool,
  triangle: triangleTool,
  hand: handTool,
};
