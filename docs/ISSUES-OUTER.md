# Known issues — upstream

Problems this app hits that a dependency has to fix — almost always
[oriui](https://github.com/markgrushevski/oriui). Each entry names the upstream id it waits on and the
local workaround it justifies, so nobody deletes the workaround as dead code. oriui reads this file as
its inbound queue. Problems we fix ourselves live in [ISSUES-INNER.md](ISSUES-INNER.md).

**How to use this file.** Newest entry first. Never work around an oriui gap by styling `.ori-*`
internals: wrap it in `apps/web/src/components/ui/`, record it here, and report it upstream
([DESIGN-SYSTEM.md](DESIGN-SYSTEM.md) §0). "Fixed upstream" is not "fixed here" — an entry stays until
the release carrying the fix is installed, then it is deleted.

Status: `confirmed` · `mitigated` (a local workaround exists) · `fixed upstream` (released or merged —
note which, then bump and delete).

---

## JP-O-13 — No list row outside OriMenu

`confirmed` · `mitigated`

- **Where:** the `/draw` menu (`apps/web/src/features/draw/SideMenu.vue`) is a panel of rows — icon, label,
  shortcut hint or chevron into a sub-panel, some of them links. oriui has no such row outside `OriMenu`, and
  `OriMenu`'s `role="menu"` cannot hold the panel's inline controls (a segmented theme picker; a select, inputs
  and a switch in the Canvas sub-panel). An `OriButton` centers its content, so a row of it needs local CSS to
  read left to right.
- **Workaround:** `apps/web/src/components/ui/MenuRow.vue` wraps `OriButton` (`variant="text"`, `fluid`) and sets
  `justify-content: flex-start` from an unlayered class of its own, which beats oriui's layered default
  (NOTES, "Unlayered CSS beats every `@layer`").
- **Upstream ask:** a list or navigation row component (icon, label, trailing hint or chevron; a button or a
  link), or a content-alignment prop on `OriButton` so a row needs no local CSS.

## JP-O-12 — An icon in a toolbar item's slot silently drops its accessible name

`confirmed` · `mitigated`

- **Where:** `OriToolbarButton` / `OriToolbarToggleItem` in rc.19 set
  `aria-label = ariaLabel ?? (label || slots.default ? undefined : tooltip)`. A filled default slot counts
  as a visible name, but our slot holds only a `ToolIcon` SVG, so the tools and undo/redo in
  `apps/web/src/features/editor/FloatingToolbar.vue` rendered with no name at all (axe `button-name`,
  critical). The tooltip became `aria-describedby` instead. The DEV warning checks the `icon` prop only,
  so it stays silent for this shape.
- **Workaround:** every item passes `aria-label` explicitly.
- **Upstream ask:** warn in DEV when the slot is filled, `tooltip` is set and neither `label` nor
  `aria-label` is; or document that a slot icon needs `aria-label`.
