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

## JP-I-03 — Pinned four oriui releases behind

`confirmed` · severity `should-fix` · source: oriUI consumer review, 2026-09-18

- **Where:** `apps/web/package.json` — `@oriui/css`, `@oriui/headless` and `@oriui/vue` all pinned exactly
  at `1.0.0-alpha.13`, while `1.0.0-alpha.17` is published and a 1.0 line is being cut.
- **What:** the exact pin is deliberate (oriui's prerelease dist-tags drift, so a range is unsafe), but the
  gap has grown to four releases and now spans a React adapter, `useTabs`/`useToast` moving into the headless
  package, `useDismissable`, and the Tier-0 accessibility fixes.
- **Timing (verified 2026-09-18 against the oriui repo):** do NOT land on `alpha.17` — it was published
  2026-07-18 and predates the pre-1.0 work now on oriui `main` (`fix/pre-1.0-tier-0`: AA tone for form
  error text, a caller's `aria-describedby` no longer dropped by text controls, toasts actually announced,
  plus packaging/dist-tag fixes), with a `1.0.0-rc` as the next publish. Several of those change the exact
  chrome a visual pass covers, so bumping now means doing that pass twice. **Wait for the rc.**
- **Fix:** upgrade to the current release as one change, with a visual pass over the toolbar, dialogs and
  the leaderboard; the upgrade is also the moment to re-check every entry in
  [ISSUES-OUTER.md](ISSUES-OUTER.md) and close the ones that shipped. Re-pin exactly, not with a range,
  until oriui reaches a stable 1.0.
