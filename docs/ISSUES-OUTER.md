# Known issues — upstream

Problems this app hits that a dependency has to fix — almost always
[oriui](https://github.com/markgrushevski/oriui). Each entry names the upstream id it waits on and the
local workaround it justifies, so nobody deletes the workaround as dead code. oriui reads this file as
its inbound queue. Problems we fix ourselves live in [ISSUES-INNER.md](ISSUES-INNER.md).

**How to use this file.** Newest entry first. Never work around an oriui gap by styling `.ori-*`
internals: wrap it in `apps/web/src/components/ui/`, record it here, and report it upstream
([DESIGN-SYSTEM.md](DESIGN-SYSTEM.md) §0). "Fixed upstream" is not "fixed here" — an entry stays until
the release carrying the fix is installed, then it is deleted, and its workaround goes in the same change: the
entry's **Remove when fixed** list says what to take out. Workaround code names its entry in a comment
(`JP-O-14`), so a search for the id finds every piece.

Status: `confirmed` · `mitigated` (a local workaround exists) · `fixed upstream` (released or merged —
note which, then bump and delete).

---

## JP-O-15 — No forced-colours support: selection and state vanish in a contrast theme

`confirmed` · `mitigated` (surfaces only)

- **Where:** in a forced-colours mode (Windows contrast themes; Playwright `emulateMedia({ forcedColors: 'active' })`)
  the browser replaces backgrounds with system colours and drops box shadows. oriui shows several states by
  background alone, so they disappear: the pressed tool in `OriToolbarToggleItem` (the `/draw` toolbar shows no current
  tool), the selected segment of `OriSegmentedControl` (the menu's theme picker), `OriSwitch` (track and thumb are
  invisible, only the label is left) and `OriSlider` (the track and fill vanish, the thumb stays as a ring). An
  `OriSurface` with `bordered` off is lifted by its shadow alone, so it has no edge at all.
- **Workaround:** surfaces only. `apps/web/src/components/ui/IslandSurface.vue` turns the hairline on when the system
  asks for more contrast or forces colours (`useThemeStore().moreContrast`), and the forced border shows. The toggles,
  segments, switch and slider have no workaround: it would mean styling `.ori-*`.
- **Remove when fixed:** once an unbordered `OriSurface` has an edge of its own in forced colours, drop
  `(forced-colors: active)` from the `moreContrast` query in `apps/web/src/core/stores/useThemeStore.ts`. The
  islands keep their hairline under `prefers-contrast: more`, and the swatches keep `forced-color-adjust: none`:
  a swatch's colour is its content, not a gap.
- **Upstream ask:** `@media (forced-colors: active)` rules in the components that show state by fill: a pressed or
  selected item in `Highlight` / `HighlightText` (or a border), the switch track and thumb and the slider track drawn
  with borders in `CanvasText` / `ButtonText`; and a transparent border on an unbordered `OriSurface`, so the forced
  border gives it an edge.

## JP-O-14 — Tooltips share one anchor name, so a bubble can open at another trigger

`confirmed` · `mitigated`

- **Where:** `OriTooltip` pairs its bubble with its trigger through `--ori-anchor`, which defaults to one shared name,
  `--ori-tooltip-anchor`. `tooltip.css` expects the bubble to find the nearest preceding trigger, but anchor
  resolution takes the *last* eligible element with the name in tree order. On `/draw` in rc.21, 27 of 33 bubbles
  measured away from their trigger: hovering the Eraser showed "Eraser — E" by the menu toggle in the top-right
  corner, and with the layers panel open the Layers tooltip sat under the delete-layer button. Tooltips from a
  `tooltip` prop (`OriToolbarButton`, `OriToolbarToggleItem`) have it too.
- **Workaround:** where we render `OriTooltip` ourselves (`IconButton`, `SwatchPicker`) it gets a name of its own
  through the documented per-instance `--ori-anchor`. The toolbar's prop tooltips can't take one, so
  `FloatingToolbar.vue` wraps each item in a `.bar__tip-scope` span with `anchor-scope: --ori-tooltip-anchor`.
  `apps/web/tests/layout/tooltips.spec.ts` checks every bubble against its trigger.
- **Remove when fixed:** the `anchor` style and its `useId` in `IconButton.vue` and `SwatchPicker.vue`, and the
  `.bar__tip-scope` spans and rule in `FloatingToolbar.vue`. Keep `tooltips.spec.ts`: it guards the fix.
- **Upstream ask:** a name per instance by default (for example `--ori-anchor` set from the bubble's id on the
  `.ori-tooltip` root), or `anchor-scope: --ori-tooltip-anchor` on `.ori-tooltip`.
