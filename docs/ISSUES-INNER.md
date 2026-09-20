# Known issues — inner (ours to fix, here)

Defects and debts whose fix lands **in this repository**. Problems that belong to a dependency — oriui
above all — live in [ISSUES-OUTER.md](ISSUES-OUTER.md).

**Boundary with the rest of `docs/`.** [IDEAS.md](IDEAS.md) is what we might *build*; [NOTES.md](NOTES.md)
is a trap we already *absorbed*; [DECISIONS.md](DECISIONS.md) is *why*; [ROADMAP.md](ROADMAP.md) is *when*.
This file is what is *broken and still open*. A closed entry leaves this file, and leaves a line in
NOTES.md only if it taught something non-obvious.

**Status vocabulary.** `confirmed` — reproduced, evidence cited · `unconfirmed` — suspected, no repro yet ·
`fixing` — a branch is open · `mitigated` — worked around, root cause open · `accepted` — will not fix, with
the reason · `fixed` — merged, note the commit.

**Protocol.** Review agents *report*; whoever orchestrates *records* here, so parallel agents never edit
this file at once. Reviewers read it first: a defect already recorded here is not a new finding.

---

## JP-I-06 — `/api/guess` degrades image laundering rather than preventing it

`accepted` · severity `low` · source: reproduced in the 2026-09-20 adversarial review

- **The claim that was too strong:** the guess route renders its raster server-side from the
  validated vector document, and we said that meant a caller could not use it to send an
  arbitrary image to Google under our API key.
- **What actually happens:** `document.ParseAndValidate` allows **5000 strokes**
  (`packages/document` / `server/internal/document`), each a `rect` with its own `fill`. A
  reviewer built a **70×70 = 4900-rect colour mosaic** (625 KB, well under the 8 MiB body cap),
  posted it to `/api/guess`, and it passed validation, rendered through the real node worker in
  **2075 ms**, and reached the model. The standalone render confirms the mosaic reproduces
  faithfully at 1024². Any picture downsampled to 70×70 is trivially identifiable by a vision
  model.
- **What still holds:** the trust boundary holds in *form* — the only bytes that leave are our
  own renderer's output of a document our own validator accepted. What does not hold is the
  *purpose* argument: laundering is **degraded in resolution, not prevented**, at the same cost
  per call.
- **Why `accepted` rather than `fixing`:** every plausible block is worse than the problem. A
  stroke-count cap for this route alone would reject legitimately detailed drawings — exactly
  the ones the feature exists for — and any threshold low enough to stop a mosaic is low enough
  to stop real art. The residual risk is small: the result is never stored, scores nothing,
  gates nothing, and an authenticated caller gets **2 a day** when a provider is configured.
- **Recorded so the claim is not re-made.** `docs/JUDGE.md` §8.3 states the honest version;
  `docs/DECISIONS.md` 2026-09-20 carries the amendment. Revisit if the per-day cap ever rises
  or the feature starts persisting anything.

---

## JP-I-05 — The zoom island overlaps the floating toolbar across the whole tablet band

`confirmed` · severity `nice-to-fix` · source: measured in a headless browser during the 2026-09-20 design
review, at eleven viewports

- **Where:** `apps/web/src/components/shell/EditorShell.vue` — `.shell__region--bottom-right` (the zoom
  island) and `.shell__region--bottom-center` (`FloatingToolbar`).
- **What:** the shell lifts the zoom island to `bottom: 4.25rem` only at `width <= 600px`, but the toolbar
  stays near-full-width well past that. Between roughly **601px and 1180px** the two overlap. Measured
  intersection: **9600px²** at 768×1024 and at 667×375 landscape, 8250px² at 840×700, 3650px² at
  1024×768, and 0 at ≤600px and ≥1200px. So every tablet, and every phone held sideways, gets zoom
  buttons sitting on top of the tool buttons.
- **Why it survived:** both breakpoints were chosen against a phone and a desktop, and the band between
  them was never measured. Nothing in CI looks at geometry.
- **Not caused by** the 2026-09-20 guess feature — that card was measured clear of both at all eleven
  sizes — but found while measuring it.
- **Fix shape:** the lift is keyed on the wrong axis and the wrong threshold. Raise the breakpoint to
  where the toolbar actually stops being full-width, and gate on available height as well as width.

---

## JP-I-04 — The sign-in form is rendered twice, into both tab panels

`confirmed` · severity `nice-to-fix` · source: live DOM inspection while building the auth modal, 2026-09-18

- **Where:** `apps/web/src/components/auth/AuthForm.vue` — the fields live in `OriTabs`'s **default**
  slot, under a two-entry `:tabs` list (`Log in` / `Register`).
- **What:** `OriTabs`'s documented API is one **named slot per tab** (`#panel-login`, `#panel-register`);
  a default slot is rendered into *every* panel. So the whole form exists twice in the DOM. Measured in
  the browser: two `.auth-form` nodes inside the one open dialog, four `<input>`s (ids `v-2`/`v-4` and
  `v-6`/`v-8`) all bound to the same two refs, and the hidden panel's submit reading `Create account`
  while the visible one reads the same — because the label follows the shared `authMode`, not the panel.
- **Impact is small and entirely invisible:** the inactive panel carries `hidden` + `display: none`, so it
  is out of the accessibility tree and out of the tab order, and the shared refs keep the two copies in
  sync. It is wasted DOM and a misused component API, not a user-facing defect — which is why it is
  recorded rather than fixed mid-slice.
- **Naming changed on `1.0.0-rc.18`:** oriui renamed `OriTabs`'s panel slots from bare `#<value>` (e.g.
  `#login`) to `#panel-<value>` (e.g. `#panel-login`, `#panel-register`) in this release. Fix candidate
  one below is written against the rc.18 names.
- **Fix (two candidates, both visual decisions):** move the fields into real `#panel-*` slots, or drop
  `OriTabs` here for the repo's own `SegmentedControl` (`components/ui/SegmentedControl.vue`) and render
  ONE form beneath it. The second is the honest shape — the two modes share every field and differ only
  in the submit label and `autocomplete` — but it changes how the form looks, so it belongs to the
  owner's UI recomposition ([IDEAS.md](IDEAS.md), post-launch list) or to the oriui rc bump, not to a
  behaviour slice.
