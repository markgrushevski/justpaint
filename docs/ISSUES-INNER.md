# Known issues — ours

Open defects whose fix lands in this repository. Problems a dependency has to fix live in
[ISSUES-OUTER.md](ISSUES-OUTER.md).

**How to use this file.** Newest entry first. An entry says where the problem is, how to reproduce or
measure it, and what the fix looks like. Delete it in the same change that fixes it; if the fix taught
something non-obvious, that goes to [NOTES.md](NOTES.md). A deliberate "won't fix" is a decision and
belongs in [DECISIONS.md](DECISIONS.md) or the relevant contract doc, not here.

Status: `confirmed` (reproduced, evidence cited) · `unconfirmed` (suspected) · `fixing` (a branch is open).

---

## JP-I-07 — An assist call longer than 30s loses its response

`unconfirmed` (read from the code, not reproduced)

- **Where:** `server/cmd/server/main.go` sets `http.Server{WriteTimeout: 30 * time.Second}`, but one
  assist attempt may take `ASSIST_TIMEOUT` (60s by default), and `GeminiAssist` can make up to two batch
  attempts, each over a transport that retries (`docs/JUDGE.md` §7). The handler runs on `r.Context()`,
  which the write deadline does not cancel.
- **Effect:** a generation that outlives 30s still runs to completion and still spends the player's
  assist allowance (it is billed before the call), but the response write fails and the client sees a
  dropped connection instead of shapes or an error.
- **To confirm:** point `GEMINI_BASE_URL` at a stub that answers after 35s and call `POST /api/assist/ops`.
- **Fix shape:** bound the whole assist request under the write deadline (a context deadline derived from
  it), or extend the write deadline for that route with `http.ResponseController.SetWriteDeadline`.
