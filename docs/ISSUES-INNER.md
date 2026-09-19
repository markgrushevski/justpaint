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

## JP-I-05 — `GeminiJudge`'s request shape is a reconstruction, unverified against the live API

`unconfirmed` · severity `blocking-for-gemini-mode` · source: building `feat/real-judge`, 2026-09-19

- **Where:** `server/internal/judge/gemini.go` — the `generateContent` wire types (`geminiRequest`,
  `geminiContent`, `geminiPart`, `geminiInlineData`, `geminiGenerationConfig`, `geminiSchema`) and
  `buildGeminiBody`/`geminiVerdictSchema`, which assemble the request Google's Generative Language API
  is supposed to accept.
- **What:** every one of those wire types was written from documentation, not exercised against the real
  API — the only thing asserting the shape today is `gemini_test.go`'s local `httptest` stand-in, which
  checks that we send what *we* think we should send, not that Google agrees. Nobody has called the real
  endpoint: there is no `GEMINI_API_KEY` yet. Three specific choices are reconstructions that would draw
  a `400` first if any one of them is wrong:
  1. **camelCase field names** (`inlineData`, `mimeType`, `responseMimeType`, `systemInstruction`) — the
     protobuf-JSON mapping this API is built on also accepts snake_case (`inline_data`, `mime_type`, …)
     for the same fields; if this endpoint/model version actually expects (or only accepts) snake_case,
     every request 400s.
  2. **`systemInstruction` sent as a top-level sibling of `contents`**, itself a `Content` value
     (`{"parts":[{"text":…}]}`) — reconstructed from general API docs, not confirmed for this exact
     endpoint.
  3. **`geminiSchema.Type` values are UPPERCASE string enums** (`"OBJECT"`, `"STRING"`, `"NUMBER"`) —
     reconstructed from the OpenAPI-subset schema docs; if the live API wants a different casing,
     `geminiVerdictSchema()` needs its four `Type` literals changed and nothing else.
- **Impact:** if any of these is wrong, `JUDGE_MODE=gemini` fails **closed**, not silently wrong — every
  duel gets a `400` inside `(*GeminiJudge).attempt`, which is the non-retryable branch
  (`resp.StatusCode >= 500` is false for a 400), so the match stays in `judging` until the stuck-judging
  sweep exhausts its retries and aborts it (`resolution: 'aborted'`, no Elo — `DECISIONS.md` 2026-09-18).
  Nothing corrupts a verdict; the failure mode is "no judge," never "wrong judge" — but it would burn
  through the abort path on every single duel until fixed.
- **Fix:** one-line per item above — flip the affected struct tag(s) to snake_case, move/rename
  `systemInstruction` if that's what's rejected, or lowercase the `Type` enum values in
  `geminiVerdictSchema()` — whichever the API's own rejection names. `geminiStatusError` already surfaces
  Google's error `message` verbatim on a non-200, so the first real call against a live key should name
  the exact field to change without anyone needing to re-derive this from the docs. Close this entry once
  a real `GEMINI_API_KEY` has scored one real duel against the live endpoint.

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
