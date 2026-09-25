# Roadmap

What is left to build. Finished work lives in git history, and the reasoning behind it in
[DECISIONS.md](DECISIONS.md). Keep this file short: add an item when it is agreed, delete it when it
ships.

## Where things stand

| Phase | Scope | Status |
|---|---|---|
| 0 | Specs: document format, API, judge contract | done |
| 1 | Go backend (auth, drawings) and a minimal Konva editor | done |
| 2 | The real vector editor: layers, undo/redo, oriui | done |
| 3 | The game: async duel, server-authoritative deadline, live WebSocket updates | done |
| 4 | Depth: ratings, AI assist, practice, "what did I draw?" — plus the open items below | in progress |
| 5 | Public release: CI, fail-fast config, rate limits, judging durability, one image | done |
| 6 | Post-launch UI | in progress |

## Open

**Game**
- **The external ML judge.** `HTTPJudge` implements [JUDGE.md](JUDGE.md) §6 and is waiting for the
  service to exist; `GeminiJudge` gives real verdicts until then.
- **Spectating.** Reconnect, presence and idle eviction are done; watching someone else's match is not.
- **Teams and tournaments** — brackets on top of `match_players`, which already generalizes past 1v1.
- **Replay** — animate a drawing from its document. Stroke order works on v1; true timing needs an
  additive field ([DOCUMENT-FORMAT.md](DOCUMENT-FORMAT.md) §9).
- **Object storage** for the judged raster and thumbnails. Until then the result screen renders the
  opponent's document client-side and `judgedImageUrl` is `null`.

**Editor**
- **Object selection, tldraw-style** — a select tool, per-stroke hit testing, marquee, move/scale and
  multi-select, all through the command stack. Mostly `packages/editor`; a phase, not a slice.
- **Editor chrome layout** — rearrange the floating islands, then make the layout hold at every
  breakpoint (`npm run test:layout` guards overlaps).

**Dependencies**
- **Next oriui release.** It renames the API vocabulary (`fill` → `solid`, `text` → `label`, …) with no
  aliases, so the bump and the call-site migration are one change: either half alone leaves buttons
  silently unstyled. Migration table: oriui's `.changeset/api-vocabulary-rename.md`. The bump closes
  JP-O-09, JP-O-10 and JP-O-11 ([ISSUES-OUTER.md](ISSUES-OUTER.md)).
