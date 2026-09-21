# Known issues — inner (ours to fix, here)

Defects and debts whose fix lands **in this repository**. Problems that belong to a dependency — oriui
above all — live in [ISSUES-OUTER.md](ISSUES-OUTER.md).

**An entry can leave sideways, not only by being fixed.** When the fix turns out to belong to a
dependency, the entry moves to [ISSUES-OUTER.md](ISSUES-OUTER.md) and keeps its history there under its
new id, so the trail lives at the destination rather than as a tombstone here.

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
