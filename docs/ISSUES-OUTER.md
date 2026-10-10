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
(`JP-O-nn`), so a search for the id finds every piece.

Status: `confirmed` · `mitigated` (a local workaround exists) · `fixed upstream` (released or merged —
note which, then bump and delete).

---

## JP-O-16 — Escape goes to the trigger's tooltip before the popover open above it

`confirmed` · `mitigated` (the test only)

- **Where:** 1.0.0-rc.23. Click a trigger that has a tooltip and opens a popover (the menu's **Custom canvas colour**
  opens `OriColorPicker`): focus stays on the trigger, without `:focus-visible`, and its tooltip counts as open even
  though no bubble shows. The first Escape is taken by the tooltip — its document listener calls `preventDefault()` in
  the capture phase — so the browser's own popover dismissal never runs and the picker stays open; a second Escape
  closes it. Measured in Playwright (`/draw`, phone drawer): with the trigger focused one Escape leaves the picker
  `:popover-open`; after `blur()` one Escape closes it. Moving the pointer away changes nothing.
- **Expected:** Escape closes the top layer first. A popover open above its own trigger is that layer, so the
  trigger's tooltip should not take the key while the popover is open — and a tooltip that is not showing should
  never take it.
- **Workaround:** none in the app. `tests/flows/draw-menu.spec.ts` "Esc in the colour picker closes only the picker"
  is marked `test.fail` until the fix ships.
- **Remove when fixed:** that `test.fail` line (search `JP-O-16`); the test then passes as written.
- **Upstream:** oriui PR #35 (`fix/tooltip-escape-layer`), due in rc.24.
