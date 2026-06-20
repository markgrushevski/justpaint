import type { Tool, ToolId } from "../types";
import { penTool } from "./pen";
import { eraserTool } from "./eraser";
import { lineTool } from "./line";
import { rectTool } from "./rect";
import { ellipseTool } from "./ellipse";
import { triangleTool } from "./triangle";

/**
 * The tool registry, keyed by {@link ToolId}.
 *
 * Each entry is a pure {@link Tool} implementing `buildStroke` (no Konva, no
 * mutable state). The editor looks tools up here by id.
 */
export const TOOLS: Record<ToolId, Tool> = {
  pen: penTool,
  eraser: eraserTool,
  line: lineTool,
  rect: rectTool,
  ellipse: ellipseTool,
  triangle: triangleTool,
};
