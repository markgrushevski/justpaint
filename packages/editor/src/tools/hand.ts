// packages/editor/src/tools/hand.ts — the hand (pan) tool.
import type { PanTool } from "../types";

/**
 * Hand — pans the view on a PRIMARY pointer drag (mouse left-drag or a
 * single-finger touch drag, which finally gives touch users a way to pan;
 * the middle-button drag keeps working with every tool).
 *
 * Deliberately data-only: a {@link PanTool} has no `buildStroke`, so the type
 * system guarantees the hand can never emit a stroke, mutate the document, or
 * push onto the undo/redo history. All the actual pan mechanics (clamp-free
 * `panBy`, the stage transform, pointer capture/abandon hardening, the
 * grab/grabbing cursor) live in the Editor's shared pan path — the same code
 * the middle-button drag uses.
 */
export const handTool: PanTool = {
  kind: "pan",
  id: "hand",
};
