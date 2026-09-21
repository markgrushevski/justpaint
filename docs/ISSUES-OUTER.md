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

Still open: JP-O-05, JP-O-06, JP-O-09, JP-O-10 and JP-O-11, below. (JP-O-04 is listed too, but its own entry
retracts it — it was never a defect. JP-O-10 was found after the bump, on 2026-09-20. JP-O-11 arrived from
ISSUES-INNER on 2026-09-21: it was filed as JP-I-04 against ourselves, and the fix turned out to be oriui's.)

**Two of those five close on one action — cutting an rc after `1.0.0-rc.18` and bumping the pin.** JP-O-09 and
JP-O-11 are both `fixed` on oriui's `main`, and neither is installable: npm's `rc` dist-tag still resolves to
`1.0.0-rc.18`, checked 2026-09-21. Nothing in this repository unblocks them.

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

## JP-O-10 — `OriButton` applies the *disabled* dim to the *loading* state, dropping a busy label to 1.68:1

`confirmed` · upstream **still unreported** · found 2026-09-20 while reviewing the AI-guess card,
re-checked 2026-09-21

- **What:** `loading` sets the native `disabled` attribute, so `.ori-button:disabled { opacity: .45 }`
  applies to a *busy* button too. Measured on our fill-primary: the "Thinking…" label lands at
  **1.68:1** in light and **2.30:1** in dark — effectively decorative, on exactly the control whose job
  at that moment is to say the app is working.
- **Why it is a defect and not a token problem:** WCAG exempts an *inactive* UI component from contrast.
  A busy control is not inactive — it is the one telling you to wait — so the exemption does not
  obviously cover it, and the dim is doing the opposite of what the state needs.
- **Suggested upstream behaviour:** skip the `.45` dim when `aria-busy="true"`, keeping `pointer-events:
  none` and the native `disabled` for input blocking. That leaves the disabled case untouched.
- **Local workaround (named, do not delete):** never let a `loading` button be the only carrier of the
  pending message. `apps/web/src/components/GuessResult.vue` states the wait in full-ink body copy
  ("It gets redrawn on the server first, so this takes a few seconds") and treats the button's own
  label as decoration.
- **Re-checked 2026-09-21 — unfixed AND unfiled.** oriui's own `packages/css/src/components/button.css`
  still reads `.ori-button:disabled, .ori-button[aria-disabled='true']:not(:focus-visible) { opacity: .45 }`
  with `.ori-button[aria-busy='true']` setting only `pointer-events: none`; the pinned rc.18 bundle is
  byte-identical on this point. No `ORI-I-*` entry in oriui's own `ISSUES-INNER.md` covers it — the
  nearest one is a different defect (`<OriButton as="a" loading>` staying keyboard-activable). **This is
  the only entry in this file waiting on nobody: it cannot be fixed upstream until it is reported
  upstream.**

## JP-O-11 — `OriTabs` cloned its fallback panel slot into every panel (arrived here as JP-I-04)

`confirmed` · upstream: oriui `ORI-I-84`, **fixed on `main`, not yet released** · filed against ourselves
2026-09-18, reclassified 2026-09-21

- **What we see here:** `apps/web/src/components/auth/AuthForm.vue` puts the fields in `OriTabs`'s
  **default** slot under a two-entry `:tabs` list, so the whole form sits in the DOM twice. Measured in
  the browser: two `.auth-form` nodes inside one open dialog, four `<input>`s bound to the same two
  refs. The inactive copy carries `hidden` + `display: none`, so it is out of the accessibility tree
  and out of the tab order, and the shared refs keep the two in sync.
- **Why it moved out of ISSUES-INNER:** it read as our misuse of the component, and it was not. The
  `#default` slot is oriui's documented per-panel fallback, scoped with that panel's tab — a template
  that ignores the scope was silently multiplied instead. oriui accepted that as a defect of the slot's
  shape (`ORI-I-84`) and fixed it on `main`: the fallback now renders into the **active panel only**.
  Upstream's own measurement found the sharp edge ours had missed — `getElementById`, and therefore
  `<label for>`, resolves to the first copy in document order, which is the **hidden** panel whenever
  the active tab is not the first.
- **What waiting costs us:** nothing a user meets. Wasted DOM, plus labels pointing into the hidden
  copy while Register is the open tab.
- **Closes on the next pin bump with no code change here** — the form becomes one instance because the
  component stops cloning it, not because we rewrote the call site. Do not hand-roll a local fix; a
  dynamic `#panel-<value>` slot name would work and would then have to be deleted again.
- **One thing to re-check after that bump:** upstream records that uncontrolled DOM state inside a
  shared template does not survive a tab switch, because the content is created fresh in the panel you
  switch into. Our fields are bound to refs declared outside the slot, so typed values survive; focus
  does not. Worth one look, not a redesign.

---

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
- **Re-checked 2026-09-21: still not installable.** npm's `rc` dist-tag for `@oriui/{vue,css,headless}`
  resolves to `1.0.0-rc.18` — exactly what we pin — so `ORI-I-85`'s fix lives on oriui's `main` and
  nowhere we can install from. There is nothing to do here until an rc ships. Note what that means
  for the guard named two bullets up: `npm run test:a11y` is **red today**, 1 failed / 6 passed,
  and the one failure IS this defect — axe reads the "Log in" label at **4.01:1** inside the open
  dialog. The suite is doing its job; it just has nothing to pass over until the fix ships. Do not
  silence it with an allowlist entry — it goes green on the bump, and a suppression added now would
  outlive the reason for it.
