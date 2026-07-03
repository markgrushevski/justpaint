/**
 * Bundle the render worker into a single self-contained ESM file that native
 * Node can run. The workspace packages (`@justpaint/editor`, `@justpaint/document`)
 * emit extensionless relative imports (fine for Vite/the browser app, NOT for
 * native Node ESM), so we bundle them in. `canvas` is a native addon and stays
 * external — resolved from node_modules at runtime. Konva is bundled but only
 * used after `konva/canvas-backend` registers node-canvas (import order in
 * render.mjs is preserved).
 */
import { build } from "esbuild";

await build({
  entryPoints: ["render.mjs"],
  outfile: "dist/render.mjs",
  bundle: true,
  platform: "node",
  format: "esm",
  target: "node20",
  external: ["canvas"], // native .node addon — cannot be bundled
  // Konva backends `require('canvas')`; ESM output has no `require`, so provide one.
  banner: {
    js: "import { createRequire as __cr } from 'module'; const require = __cr(import.meta.url);",
  },
  logLevel: "info",
});
