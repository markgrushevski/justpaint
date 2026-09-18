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

## Upstream status — 2026-09-18

**All eight entries are fixed on oriui's `main`, and NONE of it is published yet.** The next publish is
`1.0.0-rc`; until then this app stays on `1.0.0-alpha.13` and every workaround below stays in place.
Verified against the oriui working copy, not taken on report: `fix/toast-inbound-queue` and
`fix/register-sweep-1` are on `main`, and the new `align` / `pressed` / `deselectable` props exist in
the source.

**What changes for us the day we bump** (this is the bump checklist, JP-I-03):

| Entry | What lands | What we remove or gain |
|---|---|---|
| JP-O-07 | the toast queue stops stamping `closable` | our nine `closable: false` flags in `DrawView.vue` come out; **the inverse now holds** — a × requires `closable: true`. A `duration: 0` toast opts itself in, since nothing else could dismiss it |
| JP-O-08 | `align` on `OriToast`/`OriToaster` | `<OriToaster position="top-center" align="center" />` — the centring the owner asked for. It needed more than `text-align`: a centred flex child centres on the room the × leaves behind, so the fix takes the button out of flow and reserves equal space both sides |
| JP-O-01 | `OriButton` gains `pressed` (real `aria-pressed` + a pressed affordance without a toolbar ancestor) | the tool picker announces its selection to assistive tech; and it was NOT done via the ungating trap we warned about |
| JP-O-02 | `deselectable: false` on the toolbar toggle group | our hand-rolled re-selection guard comes out |
| JP-O-03 | `OriPopover` splits the panel role from the trigger hint | the cast at the call site comes out |
| JP-O-04 | recorded `accepted` — the docs were right, our entry was wrong | nothing |
| JP-O-05 | the à-la-carte import line is documented per component page | `scripts/check-styles.mjs` stays: docs are not a gate |
| JP-O-06 | structural hairlines derive from `currentcolor`; still no public neutral token | keep our `--jp-color-outline` |


## JP-O-01 — A toggle button has no accessible pressed state, and looks identical to hover

`mitigated` · upstream: oriui `ORI-I-41` (and `ORI-I-10`) · confirmed 2026-09-18

- **What:** `OriButton`'s `active` prop is a look, not a state: it emits `data-active` and no
  `aria-pressed`, so a tool that is currently selected announces nothing to assistive tech. Worse, the
  `[data-active]` paint is the same treatment as `:hover`, so a sighted user cannot tell the selected tool
  from the one under the cursor either. The one correct pressed treatment in oriui (tint plus inset
  hairline) is gated behind a `.ori-toolbar` ancestor.
- **Where it bites us:** the tool picker in `apps/web/src/components/FloatingToolbar.vue`.
- **Careful:** the obvious upstream fix — ungating that rule to `.ori-button[aria-pressed='true']` — is
  **wrong**, and oriui has it recorded as `ORI-I-61`: the rule was authored for the toolbar's `text`
  variant and would strip the background from every filled or tonal toggle. Do not push for it.

## JP-O-02 — A single-select toolbar toggle group cannot require a selection

`mitigated` · upstream: oriui `ORI-I-48` · confirmed 2026-09-18

- **What:** `useToolbarToggleGroup` with `type: 'single'` is unconditionally deselectable — clicking the
  active item clears it. A tool picker must always have exactly one tool selected.
- **Workaround that exists because of this:** the toolbar re-selects by hand rather than letting the group
  own its state. Do not "simplify" that guard away.

## JP-O-03 — `OriPopover`'s `role` prop forces a cast at the call site

`mitigated` · upstream: oriui `ORI-I-44` · confirmed 2026-09-18

- **What:** `role` is typed as an unconstrained `string`, so the trigger slot's prop bag infers
  `'aria-haspopup': string`, which does not type-check against Vue's `ButtonHTMLAttributes` literal union.
  Consumers have to cast the bag away to bind it.
- **Note:** the narrow fix (restricting `role` to the `aria-haspopup` union) would forbid legitimate panel
  roles; upstream is tracking the correct split.

## JP-O-04 — `OriTooltip` renders an `aria-describedby` its own docs call non-functional

`unconfirmed` here · upstream: oriui `ORI-I-46` · raised 2026-09-18

- **What:** the component emits a describedby wiring that the documentation says does not work, and the
  working path is a different API we never wired. Whether any tooltip in this app is actually announced is
  untested — verify before assuming either way.

## JP-O-05 — À-la-carte `@oriui/css/components/*.css` has no completeness guard

`mitigated` · upstream: oriui `ORI-I-40` · confirmed 2026-09-18

- **What:** importing one stylesheet per component is the documented way to pay only for what you use, but
  nothing tells you when the list has fallen behind the components you render — the failure mode is a
  silently unstyled component, with no console warning and no build error. It already happened here
  ([ISSUES-INNER.md](ISSUES-INNER.md) JP-I-01).
- **Our half:** the guard test described in JP-I-01. Upstream's half: documenting the import line on each
  component page, and stating that the full bundle is the default for anything but a size-critical app.

## JP-O-06 — No neutral/structural colour token, so borders and hairlines have no supported handle

`mitigated` · upstream: oriui `ORI-I-43` · confirmed 2026-09-18

- **What:** oriui derives structural neutrals inline (roughly forty ad-hoc `color-mix` percentages, and not
  consistently — `--ori-color-on-surface 12%` in one block, `currentcolor 12%` in another), and exposes no
  token for "the colour of a divider, hairline or outline". A consumer that wants one consistent structural
  colour has nothing to read.
- **Workaround that exists because of this:** the app declares its own outline colour — currently under
  oriui's prefix, which is its own bug (JP-I-02). Renaming it to `--jp-*` is the right move regardless;
  if oriui later ships a real neutral token, the app repoints its own token at it in one line.

## JP-O-07 — The toast queue forces a close button the component itself defaults off

`mitigated` · upstream: not yet filed · confirmed 2026-09-18 (`@oriui/vue` 1.0.0-alpha.13)

- **What:** `OriToast` declares `closable` with a `false` default and renders the × behind a `v-if`, which
  is right. But `useToast`'s queue stamps `closable: true` onto every toast it enqueues
  (`dist/components/toast/use-toast.js`), so the component default is unreachable: a caller who says
  nothing gets a dismiss button. The two defaults disagree, and the queue wins.
- **Where it bites us:** every toast in `apps/web/src/views/DrawView.vue`. Removing our `closable: true`
  flags changed nothing; only an explicit `closable: false` per call site removes the ×.
- **Ask:** let the queue leave `closable` undefined so the component default applies, or document that the
  queue is the authority and change its default to false.

## JP-O-08 — A toast has no way to centre its text

`accepted` · upstream: not yet filed · confirmed 2026-09-18

- **What:** `.ori-toast__text` is `text-align: start` with no prop or token to change it. In a
  `position="top-center"` toaster carrying one-line status messages ("Saved.", "Copied image"), the text
  hugs the left of a fixed-width box and reads as misaligned rather than as a centred status bar.
- **Where it bites us:** `OriToaster position="top-center"` on `/draw`.
- **Ask:** a toast-level alignment choice — either a prop, or centring when the toast has a single text
  child and no title/action/close. Not urgent; we are not overriding it locally, because a consumer
  restyling a vendor component is exactly what `docs/DESIGN-SYSTEM.md` forbids.

## JP-O-09 — `OriDialog` dims its whole body, dropping the primary button and hints below AA

`confirmed` · upstream: not yet filed · measured 2026-09-18 against `1.0.0-alpha.13`, **still present on `1.0.0-rc.18`**

- **What:** `.ori-dialog__body { opacity: 0.85 }` (`packages/css/src/components/dialog.css`) dims everything
  slotted into a dialog — not just captions, but the primary action's label too. Inside a field, it
  compounds with `.ori-field__hint { opacity: 0.7 }` for an effective 0.595.
- **Measured in a real browser** on the sign-in modal (light theme, `--ori-color-surface` `rgb(240 242 246)`):

  | element | raw | composited | WCAG AA |
  |---|---|---|---|
  | `OriButton variant="fill" color="primary"` label, 16px/400 | 5.43:1 | **4.00:1** | needs 4.5:1 |
  | `.ori-field__hint`, 12.8px/400 | 13.72:1 | **3.95:1** | needs 4.5:1 |

  Dark theme passes — this is a light-theme-only failure, which is exactly how it survived: the token
  pair itself is AA (5.43:1) and only loses AA once the dialog dims it.
- **Why it is yours, not ours:** `docs/DESIGN-SYSTEM.md` bars us from restyling a vendor component's
  internals, and the fix belongs upstream anyway — an ambient dim is right for supporting text and wrong
  for an interactive control. Either scope the opacity to the dialog's descriptive content, or drop it and
  express the hierarchy with a token tone that is AA by construction.
- **Why no local workaround yet:** overriding `.ori-dialog__body` would be exactly the vendor-restyling the
  design system forbids, and every consumer of every oriui dialog has the same bug. Recorded and reported
  instead. Our own guard against a recurrence is a test, not a CSS override: `apps/web/tests/a11y/draw.spec.ts`
  now opens the sign-in dialog and runs axe over it.
- **Note for the bump:** `scripts/check-contrast.mjs` cannot catch this class of defect — it reads token
  pairs out of `main.css` and knows nothing about a component's runtime opacity. Only a rendered audit does.
- **Also affected here:** `ConfirmDialog`'s `color="danger"` Confirm button, same mechanism, predates this.
