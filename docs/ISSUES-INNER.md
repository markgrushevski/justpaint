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

## JP-I-01 — OriBadge and OriSkeleton render unstyled

`confirmed` · severity `blocker` · source: oriUI consumer review, 2026-09-18

- **Where:** `apps/web/src/main.ts` (the à-la-carte oriUI import block) against
  `apps/web/src/views/LeaderboardView.vue:14,91,94` and `apps/web/src/components/game/JudgingOverlay.vue:12,26,30`.
- **What:** the app imports nineteen `@oriui/css/components/*.css` files by hand, and `badge.css` and
  `skeleton.css` are not among them — while `OriBadge` and `OriSkeleton` are both rendered (the leaderboard
  and the judging overlay). Both therefore ship with no block styles at all: the skeleton placeholders have
  no shimmer and no shape, the badge no chip.
- **Fix:** add the two imports, then close the class of bug rather than the instance — a small test that
  collects every `Ori*` component imported anywhere under `apps/web/src` and asserts a matching
  `@oriui/css/components/<name>.css` import exists in `main.ts`. The hand-maintained list is the defect;
  the two missing lines are only its first symptom. See [ISSUES-OUTER.md](ISSUES-OUTER.md) JP-O-05 for the
  upstream half.

## JP-I-02 — The app defines `--ori-color-outline`, squatting oriui's token namespace

`confirmed` · severity `should-fix` · source: oriUI consumer review, 2026-09-18

- **Where:** `apps/web/src/main.css:50,87` (declares it per theme), consumed at
  `apps/web/src/components/FloatingToolbar.vue:309,383,423,449` and
  `apps/web/src/components/game/OpponentStatusChip.vue:120`.
- **What:** `--ori-color-outline` does not exist in `@oriui/css` — the app invented it inside oriui's
  reserved `--ori-*` prefix. It works today only because nothing upstream defines it. The day oriui ships a
  token by that name with different semantics, every border and hairline in the floating toolbar silently
  changes, and the failure will look like an oriui regression rather than an app bug.
- **Fix:** rename to `--jp-color-outline` (app namespace) and keep the same per-theme values. This is worth
  doing whether or not oriui ever adds a neutral/structural token — see JP-O-06.
- **Rule this restates:** `docs/DESIGN-SYSTEM.md` §0 — consume oriui through its API, never by writing into
  its namespace.

## JP-I-03 — Pinned four oriui releases behind

`confirmed` · severity `should-fix` · source: oriUI consumer review, 2026-09-18

- **Where:** `apps/web/package.json` — `@oriui/css`, `@oriui/headless` and `@oriui/vue` all pinned exactly
  at `1.0.0-alpha.13`, while `1.0.0-alpha.17` is published and a 1.0 line is being cut.
- **What:** the exact pin is deliberate (oriui's prerelease dist-tags drift, so a range is unsafe), but the
  gap has grown to four releases and now spans a React adapter, `useTabs`/`useToast` moving into the headless
  package, `useDismissable`, and the Tier-0 accessibility fixes.
- **Fix:** upgrade to the current release as one change, with a visual pass over the toolbar, dialogs and
  the leaderboard; the upgrade is also the moment to re-check every entry in
  [ISSUES-OUTER.md](ISSUES-OUTER.md) and close the ones that shipped. Re-pin exactly, not with a range,
  until oriui reaches a stable 1.0.
