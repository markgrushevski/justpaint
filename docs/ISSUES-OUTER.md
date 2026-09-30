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
