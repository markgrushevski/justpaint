import { BRUSH_DEFAULTS } from "@justpaint/document";
import type { ToolStyle } from "./types";

/**
 * The editor's default drawing style. `color`/`fill`/`strokeWidth` live here
 * (not in the document) so defaults are identical everywhere — the old
 * dev/prod default-color divergence (DOCUMENT-FORMAT §5.2) must not recur.
 */
export const DEFAULT_STYLE: ToolStyle = {
  color: "#1b1b1b",
  fill: null,
  strokeWidth: 4,
  brush: BRUSH_DEFAULTS,
};
