# Known issues — outer (a dependency has to fix these)

Problems this app hits that **cannot be fixed here** — almost all of them in
[oriui](https://github.com/markgrushevski/oriui), which the same owner maintains. We record the workaround
we carry locally and the upstream id it is waiting on. Problems we own are in
[ISSUES-INNER.md](ISSUES-INNER.md).

**How this file is used upstream.** oriui treats this list as its inbound queue: an accepted entry becomes
an `ORI-I-*` entry in oriui's own `ISSUES-INNER.md`, which is where the fix is tracked. So each entry here
carries the upstream id, and the only thing this file tracks afterwards is whether the released fix has
landed in our pinned version. One home for the fix, one home for the workaround.

**Status vocabulary.** `confirmed` — reproduced here · `unconfirmed` — suspected · `mitigated` — a local
workaround exists (named, so nobody deletes it as dead code) · `accepted` — a constraint we design around ·
`fixed upstream` — released; note the version, upgrade, then delete the entry.

**Rule:** never work around an oriui gap by reaching into `.ori-*` internals. Wrap it in
`apps/web/src/components/ui/`, record it here, and tell the owner — that is `docs/DESIGN-SYSTEM.md` §0.

---

## Bump landed — 2026-09-18 (`1.0.0-alpha.13` → `1.0.0-rc.18`)

`apps/web` now pins `@oriui/{vue,css,headless}` to **exact** `1.0.0-rc.18` (no `^` — a caret range would
also match a future stable `1.0.0`, and the `rc` dist-tag itself moves). All gates are green: prettier,
vue-tsc, build, vitest, stylelint, eslint, check-contrast, check-styles. Every workaround below was removed
and re-verified live in a real browser, not taken on the changelog. Full rationale for the exact pin and
for the one thing we deliberately did not adopt: [DECISIONS.md](DECISIONS.md) 2026-09-18.

Fixed upstream and removed (released, upgraded, entry deleted — per the rule above):

- **JP-O-01** — `OriButton` gained `pressed`. `IconButton`'s hand-wired `:aria-pressed="active || undefined"`
  is gone; the prop is renamed `active` → `pressed` and forwarded on the four `/draw` panel toggles
  (shortcuts, layers, assist, menu). Measured: the layers toggle renders `aria-pressed="true"` open /
  `"false"` closed — the off state is announced at all, which the hand-wired version never emitted.
- **JP-O-02** — `OriToolbarToggleGroup` gained `deselectable`. `FloatingToolbar.vue` passes
  `:deselectable="false"`; the hand-rolled re-selection guard is gone (the surviving `if` is only type
  narrowing on the emit's `string | string[] | undefined` signature).
- **JP-O-03** — `OriPopover`'s trigger slot types cleanly now. The `v-bind="popoverTrigger as
  Record<string, unknown>"` cast at the call site is gone; plain `v-bind="popoverTrigger"`, vue-tsc clean.
- **JP-O-07** — the toast queue stopped stamping `closable`. All eight `closable: false` flags in
  `DrawView.vue` are gone. The inverse now holds — a × requires `closable: true`, and a `duration: 0`
  toast opts itself in since nothing else could dismiss it. Measured: no close button renders.
- **JP-O-08** — `align` landed on `OriToast`/`OriToaster`. `<OriToaster position="top-center"
  align="center" />` now renders `ori-toast_align-center`, computed `text-align: center`. Measured: the
  text sits 2px off the card's centre (a leading icon) — against the visible skew the owner raised
  2026-09-18. Nothing local to remove here: this one was `accepted`, never overridden.

Still open after the bump: JP-O-04, JP-O-05, JP-O-06 and JP-O-09, below.

## JP-O-04 — `OriTooltip`'s `aria-describedby`: entry retracted, not a defect

`accepted` · upstream: oriui `ORI-I-46` (no defect found) · retracted 2026-09-18

- **What:** re-reviewed during the `1.0.0-rc.18` bump: oriui's docs are right and our original report was
  wrong — there is no describedby defect in `OriTooltip`. Recorded so the retraction stays on the record
  rather than the entry silently vanishing and the same non-issue getting re-filed later.

## JP-O-05 — À-la-carte `@oriui/css/components/*.css` has no completeness guard

`mitigated` · upstream: oriui `ORI-I-40` · confirmed 2026-09-18, still open on `1.0.0-rc.18`

- **What:** importing one stylesheet per component is the documented way to pay only for what you use, but
  nothing tells you when the list has fallen behind the components you render — the failure mode is a
  silently unstyled component, with no console warning and no build error. It already happened here.
- **Upstream's half, done:** `1.0.0-rc.18`'s docs now state the import line per component page, and that
  the full bundle is the default for anything but a size-critical app.
- **Our half, still load-bearing:** `apps/web/scripts/check-styles.mjs` stays — docs are not a gate, and
  nothing upstream stops the list from silently drifting again. Rebuilt during this bump anyway: it
  hardcoded a path into the workspace-root `node_modules`, and npm hoisted `@oriui/css` into
  `apps/web/node_modules` this time, so it died with ENOENT on a package that was actually present. It now
  resolves via `createRequire(import.meta.url).resolve('@oriui/css/package.json')`, which doesn't care
  where npm hoisted to.

## JP-O-06 — No neutral/structural colour token, so borders and hairlines have no supported handle

`accepted` · upstream: oriui `ORI-I-43`, shipped `--ori-color-outline` / `--ori-color-outline-strong` in
`1.0.0-rc.18` · decision reconfirmed 2026-09-18

- oriui now exposes a public token, closing the literal request — but we deliberately do not adopt it.
  `--ori-color-outline` is `color-mix(in srgb, currentcolor 12%, transparent)` (`-strong` is 28%), a
  hairline tint that follows text colour, while our `--jp-color-outline` is a fixed per-theme colour held
  to the 3:1 non-text bar by `scripts/check-contrast.mjs` — currently 3.07:1 / 3.44:1 / 3.83:1 / 4.08:1
  against surface and background in both themes. A currentcolor tint cannot satisfy that gate and was
  never meant to — a different axis, as oriui's own docs say. Same name, different job; full rationale in
  [DECISIONS.md](DECISIONS.md) 2026-09-18. No workaround needed beyond keeping our own token, and there is
  no longer a naming collision to track either: ours already lives under its own `--jp-*` prefix.

## JP-O-09 — `OriDialog` dims its whole body, dropping the primary button and hints below AA

`confirmed` · upstream: accepted, fixed on oriui `main`, not yet released · measured 2026-09-18 against
`1.0.0-alpha.13`, **still present on `1.0.0-rc.18`**

- **What:** `.ori-dialog__body { opacity: 0.85 }` (`packages/css/src/components/dialog.css`) dims everything
  slotted into a dialog — not just captions, but the primary action's label too. Inside a field, it
  compounds with `.ori-field__hint { opacity: 0.7 }` for an effective 0.595.
- **Measured in a real browser** on the sign-in modal (light theme, `--ori-color-surface` `rgb(240 242 246)`):

  | element | raw | composited | WCAG AA |
  |---|---|---|---|
  | `OriButton variant="fill" color="primary"` label, 16px/400 | 5.43:1 | **4.00:1** | needs 4.5:1 |
  | `.ori-field__hint`, 12.8px/400 | 13.72:1 | **3.95:1** | needs 4.5:1 |

  Dark theme passes — this is a light-theme-only failure, which is exactly how it survived: the token
  pair itself is AA (5.43:1) and only loses AA once the dialog dims it. Re-measured 2026-09-18 against
  `1.0.0-rc.18`: unchanged.
- **Why it is yours, not ours:** `docs/DESIGN-SYSTEM.md` bars us from restyling a vendor component's
  internals, and the fix belongs upstream anyway — an ambient dim is right for supporting text and wrong
  for an interactive control.
- **Upstream status, updated 2026-09-18 — accepted, fixed on `main`, not yet released:** oriui removed
  `.ori-dialog__body { opacity: 0.85 }` outright. They independently reproduced the defect across eight
  skins and both themes, matched our numbers to two decimals, and found worse cases than ours: **3.12:1**
  on `luxury · light · button-fill`, 3.35:1 on `tech · light`, and 4.14:1 on `button-danger` across six
  skins — which also covers our own `ConfirmDialog`'s `color="danger"` button. After their fix, the worst
  reading inside a dialog, across 128 readings, is 4.87:1. It ships in the next rc after `1.0.0-rc.18`.
  **Closes on our next pin bump with no code change on our side** — pure CSS, no markup or API change —
  but the dialog body will read slightly more contrasty, so give it one visual look after that bump.
- **Why no local workaround yet:** overriding `.ori-dialog__body` would be exactly the vendor-restyling the
  design system forbids, and every consumer of every oriui dialog has the same bug. Recorded and reported
  instead. Our own guard against a recurrence is a test, not a CSS override: `apps/web/tests/a11y/draw.spec.ts`
  now opens the sign-in dialog and runs axe over it.
- **Note for the bump:** `scripts/check-contrast.mjs` cannot catch this class of defect — it reads token
  pairs out of `main.css` and knows nothing about a component's runtime opacity. Only a rendered audit
  does. When the fix's release lands: re-measure, then delete this entry, and give `ConfirmDialog`'s
  `color="danger"` button the same once-over — upstream's own six-skin `button-danger` numbers say it is
  the same mechanism.
- **Also affected here:** `ConfirmDialog`'s `color="danger"` Confirm button, same mechanism, predates this.
