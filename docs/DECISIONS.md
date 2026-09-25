# Decision log

Key decisions and the reasons behind them, newest first. Each entry states a decision that still stands. The mechanics live in the contract docs each entry points to.

## 2026-09-21 — A rendered-geometry test layer beside the rendered-a11y one

- **Why:** the zoom island overlapping the bottom toolbar passed every existing gate. vue-tsc sees types, Vitest renders into happy-dom (no layout), stylelint reads declarations, axe reads the accessibility tree. None of them can see two boxes painted on top of each other, and only a rendered browser can.
- **What:** `apps/web/tests/layout/chrome-overlap.spec.ts`, a second Playwright suite sharing one config with `tests/a11y`, run by `npm run test:layout -w @justpaint/web`. Like `test:a11y`, it is a local gate rather than a CI one, because both need a running dev server.
- **The test asserts the invariant, not the threshold.** The CSS lift (`@media (width <= 1200px)` in `EditorShell.vue`) is arithmetic over today's toolbar and island widths and will change. The test renders the real page at eleven viewports (both sides of the 600px phone breakpoint and of the 1200px threshold, plus a landscape phone) and asserts that no two of the shell's three bottom regions overlap. It measures each region's first element child, because the centre region is a full-width strip.
- **Checked against the bug:** with the old CSS restored, 6 of the 11 cases fail, so the suite does not pass vacuously.

## 2026-09-20 — A model per kind, a quota pool per model, and a real assist impl

Contracts: `docs/ASSIST.md` §3, `docs/GAME.md` §4.3, `docs/JUDGE.md` §8.1.

- **`AI_MODEL_PER_KIND` overrides `GEMINI_MODEL` per kind**, in the same `kind=value` list that `AI_DAILY_PER_USER` takes and through the same parser. The kinds differ in difficulty: the duel judge's number feeds Elo and has to be the steadiest, while the guesser's mistakes cost nothing. One model for all of them either overpays or underserves. An unknown kind name is a boot error, and the resolved model per kind is logged at boot, because an override that silently didn't apply keeps working on the default.
- **The global ceiling is counted per provider and model** (`google:<model>`, `aibudget.Provider.WithModel`). Google's free tier meters per project per model: a `429` body named the quota `GenerateRequestsPerDayPerProjectPerModel-FreeTier`, value 20. A single `google` counter would add independent pools together and refuse calls against a budget neither had spent. A provider that isn't metered per model, such as the external ML judge's service, keeps its bare name.
- **`AI_DAILY_GLOBAL` defaults to 15**, set against that measured 20/day. On a free key the provider's own ceiling usually binds first. The ledger records one row per judging pass, and a pass may retry up to 3 times, so 15 rows can mean up to 45 requests. What the number buys is that one player cannot spend the day's quota in a burst. Raise it on a paid key.
- **Assist's real impl is `GeminiAssist` (`ASSIST_MODE=gemini`).** It shares the judge's key, quota and HTTP client, so there is one vendor, one quota and one client to maintain. `FakeAssist` stays the default. Assist has no model knob of its own, because its model is also its budget key. The seam stays an interface so the external ML can take it later.
- **The model emits primitive shapes, not `Op`s.** It returns a flat list of `rect`/`ellipse`/`polygon`/`line`, and Go expands that into the `add_layer` + `add_stroke` batch. The model never invents an id or names a layer, so duplicate-id and unknown-layer failures can't happen. What's left for it to get wrong (geometry, colour) fails in `ValidateOpBatch` and earns the one retry. Points travel as a flat `[x1,y1,x2,y2,…]` integer array, which has one failure mode we can name exactly: an odd number of values.
- **Three schema settings fix observed failures** (details in `docs/NOTES.md`):
  - Coordinates are `INTEGER`. At temperature 0, a `NUMBER` field produced an endless run of zeros.
  - Every shape field is `required`. With only `type` required, the model repeated three-field stubs.
  - `maxOutputTokens` is 8192. When a thinking model runs out of tokens mid-list, the API closes the JSON so it still parses, and the truncated last shape looks like a bad drawing.
- **`ASSIST_TIMEOUT` (60s) is separate from `JUDGE_TIMEOUT` (10s).** Composing shapes takes 4–9s when healthy and far longer under load. Raising the shared knob would push the duel's retry envelope past `game.JudgePassBudget`. A loose bound costs a stuck goroutine for a minute. A tight one makes the feature fail whenever the provider is slow.

## 2026-09-20 — One AI-call ledger: a daily budget per kind and per provider

Every AI feature spends against one table, `ai_calls` (migration `00007`, `server/internal/aibudget`). Counting by querying each feature's own tables couldn't cover a guess, which leaves no row anywhere, or assist, whose only ceiling was an in-process token bucket reset by every deploy and every wake from idle. Mechanism: `docs/GAME.md` §4.3. HTTP shape: `docs/API.md` §3.1.

- **Two row shapes, one table.** A provider row (`user_id` null) records that a request is about to be made. A player row (`provider` null) records that this player was granted a round. A duel costs the provider one request per judging pass but costs two players an allowance each, so a single combined row would either double-count or undercount. `kind` takes no check constraint or enum. It is a Go constant, so the compiler catches a typo, and a new AI feature needs no migration.
- **Each fact is billed when it happens.**
  - Duel: the two player rows are written in the matchmaking transaction at `open → drawing`. A provider row is written at every entry into `judging` (`game.Service.enterJudging`: the last submit, the deadline path, the stuck-judging re-fire). A forfeited or abandoned duel therefore bills no provider row, and each re-fire bills again. The judge's own retries inside one pass stay uncounted, because counting them would give the frozen `Judge` contract a database dependency.
  - Practice, guess and assist: billed outside any transaction, after the server-side render and before the provider call. The render spends nobody's quota. A provider call that failed still spent quota, and a rolled-back record would hide a broken provider draining it.
- **The per-user cap is enforced by the INSERT.** The check before the work is two unlocked reads, so under a burst every caller passes: a cap of 2 was measured billing 24 of 25 concurrent requests. `RecordAICallUnderCap` writes the rows only while the count is under the cap, and writing none is the refusal. It is still not exact under `READ COMMITTED`, but the window shrinks to one statement. The global half stays advisory (read before the work, never gating a write), because refusing a write after the work is done would strand it, not prevent it.
- **Per-user is per kind.** Defaults: duel 20, practice 20, guess 2, assist 40 (`aibudget.DefaultPerUser`). The features aren't substitutes for each other, so one shared allowance would be wrong for at least one of them. One player can now reach more of the global ceiling, which is what the global half is for.
- **Global is per provider.** Google running dry must never refuse a feature backed by a different provider. Which provider backs a kind is resolved at the composition root from the impls actually built (`assist.CallsProvider`, read once by `aibudget.Policies`), not from a second reading of the mode envs. A mode says which impl was asked for, and only the impl knows whether it calls anybody. Reading the mode once billed a stub impl for calls it never made. A kind with no provider (a fake) is never checked or recorded.
- **Refusals.** Every per-kind refusal names the player's own cap from one template (`msgKindSpentFmt` + `Kind.Noun`), e.g. *"you have used all 20 of your duels for today…"*. The player could count that number themselves. The global refusal discloses nothing, because the budget's size is operator information. When the provider's own quota runs out (`judge.ErrQuotaExhausted`), practice, guess and assist answer the same `429` as the global refusal. A `500` would invite a retry that can't succeed until the provider's window rolls over.
- **Config.** `AI_DAILY_GLOBAL` + `AI_DAILY_PER_USER` (a `kind=n` list). `JUDGE_DAILY_BUDGET` still works as an alias of `AI_DAILY_GLOBAL`. Setting both to different values is a boot error. `JUDGE_DAILY_PER_USER` seeds only `duel` and `practice`, never a kind added later. Values below 1 and unknown kinds are boot errors. An allowance for a kind whose impl calls nobody is inert and logged as a warning.
- **Retention.** Only the last 24h is read, but rows are kept a week, so a refusal from last week can still be explained. The sweep runs hourly and once at boot, because a free-tier instance restarts more often than hourly. There was no backfill: counters started at zero, and the worst case was one extra day's allowance.

## 2026-09-20 — "What did I draw?": a third vision seam, a server-rendered raster, nothing stored

A `/draw` button asks the AI what the canvas depicts (`POST /api/guess`, `internal/guess`, `judge.Guesser`). Contract: `docs/API.md` §13, `docs/JUDGE.md` §8.3.

- **A third seam, not `Judge` and not `Critic`.** `Judge` is frozen and compares two images. `Critic` scores one drawing against the prompt it was drawn for, and on `/draw` there is no prompt. `Guesser` answers "what is this". Its `confidence` and a critique's `score` both range over [0,1], but they are not comparable.
- **The raster is rendered server-side from the validated document.** Accepting a client PNG would make the route an open pipe to a third-party model under our key. This limits what the route accepts, but not what the picture shows: 4900 filled rectangles (inside the 5000-stroke cap) pass the validator as a mosaic. So it raises the cost and lowers the fidelity of laundering an image rather than preventing it (`docs/JUDGE.md` §8.3). A 4 MiB raster guard also sits on the seam.
- **`document.ParseAndValidate`, not the duel's validator.** The square 1080² rule belongs to the duel, where two drawings are compared. A free-draw canvas can be any size the format allows.
- **Nothing is persisted.** The drawing may never be saved, and a guess scores, ranks and gates nothing. The ledger records that a call was made, and that is the only durable fact.
- **2 guesses per player per day**, the smallest allowance of any kind. Otherwise a novelty question would be the cheapest way to empty the provider's quota.
- **`JUDGE_MODE=http` answers a `500` that names the cause** instead of falling back to the fake. The external service has no endpoint that names one drawing, and a fake guess is a more convincing lie than a fake score: a wrong guess reads as the feature working.
- **Prompt injection is narrowed, not closed.** No player text enters the prompt, so pixels are the only untrusted channel. The instruction treats everything in the image as drawing, with the carve-out that writing may be the drawing. What actually bounds the risk is that a guess affects nothing and is stored nowhere.
- **UX.** The guess button is an icon in `/draw`'s top-left island, because assist owns the top-centre slot. It is disabled on an empty canvas and toggles the card. Re-asking happens only from the card's "Guess again", where the cost is visible. Every outcome, including a `429`, renders calmly in one `OriSurface` card. With two a day, "that was your last one" is an expected outcome, not an error.

## 2026-09-20 — Single-player practice: its own table, its own seam, the same budget

A duel needs two people online at once. With no player base, a lone visitor sat out the 10-minute open-match TTL and got an abandoned match. Practice (`/practice`, `internal/practice`, migration `00006`) scores one drawing using the prompts, render worker and judge that already existed. Contract: `docs/API.md` §12, `docs/GAME.md` §10, `docs/JUDGE.md` §8.2.

- **Not a `matches` row.** The duel lifecycle (matchmaking, a shared deadline, forfeit, abandonment, the judging watchdog) exists because two people wait on each other. Its deadline logic expects two submissions, and a single-seat round would wedge in `judging`. `practice_runs` is a flat table with no status, no sweeper and no deadline.
- **A separate `Critic` seam.** Scoring one drawing inside the frozen, comparative `Judge` contract would mean sending the same image twice or widening the contract. `judge.Critic` is ours, has fake and Gemini impls, and is selected by the same `JUDGE_MODE`.
- **`JUDGE_MODE=http` leaves practice unconfigured.** Every run answers a `500` naming the cause (*"the configured judge cannot score a single drawing"*). A fake critique presented as real would be a lie the player can't detect.
- **Practice uses the same daily budget as the duel, under its own `practice` kind.** The attempt row is written before the render and critique, and the verdict is stamped afterwards, deliberately not in one transaction: a failed critique still spent quota.
- **No Elo.** Rating a run nobody else played would let a player farm rating alone. `practice_runs` has no rating columns, and the leaderboard never reads it.

## 2026-09-19 — A daily AI-call budget: per-player checked first, global second

Once a duel could be scored by a real model, every duel spent a metered resource that the per-IP rate limiter doesn't protect. The limiter bounds request rate: the write tier allows 30 writes a minute and a duel is about four of them. The provider's binding limit is requests per day, so one IP could empty a day's quota within minutes, after which nobody's duels get judged. How calls are counted: 2026-09-20. Current rule: `docs/GAME.md` §4.3.

- **Two halves: a per-player cap and a global cap.** A global cap alone lets one abuser deny everyone. A per-player cap alone doesn't protect the budget. The per-player check runs first, so a capped player is told about their own limit and never about the service's remaining budget.
- **A rolling 24h window, not a calendar day.** A calendar day needs a timezone and hands out a full allowance at one fixed instant, which is exactly when a burst can empty it. Staying under the ceiling in every 24h window also keeps us under it in whatever calendar day the provider counts.
- **No "unlimited" sentinel.** Values below 1 are rejected at boot. An operator who wants effectively no limit sets a large number. A per-player cap above the global one is legal and means the global cap is the only ceiling.
- **Not enforced when the impl calls nobody** (the fakes). There is no quota to protect in dev or CI, and a ceiling on the local loop would be a bug.

## 2026-09-19 — A vision LLM as the interim real judge; a named judging-pass budget

`JUDGE_MODE=fake|http|gemini`, default `fake`. `config.Load` fails fast when a mode's dependency is missing (`JUDGE_BASE_URL` for `http`, `GEMINI_API_KEY` for `gemini`). Contract: `docs/JUDGE.md` §7/§8.1.

- **`GeminiJudge` is the real judge until the external ML is ready.** Otherwise the product would ship `FakeJudge`'s ink-coverage verdict indefinitely. It actually reads the prompt and both pictures, and it swaps for `HTTPJudge` with no game-loop change, which is what the `Judge` seam is for.
- **One call per duel, both rasters together.** The free tier limits requests per day, so scoring each drawing separately would halve the number of duels. One call also lets the model actually compare the two drawings.
- **Clamp vs reject.** An over-long `reason` is clamped by `GeminiJudge` but rejected by `HTTPJudge`. Verbosity from a model we prompt is our problem to absorb. The same violation from a peer service is a contract break that shouldn't be hidden. Scores and `winner` are validated strictly in both and never clamped.
- **`game.JudgePassBudget` (60s) bounds one judging pass.** A flat 30s pass timeout was incompatible with 3 judge attempts of 10s each: the third attempt could never finish once the renders had run. The budget is named, and boot warns when `3 × JUDGE_TIMEOUT` doesn't fit inside it (`docs/NOTES.md`).
- **`GEMINI_MODEL` / `GEMINI_BASE_URL` are configurable, and the default model is pinned** (`gemini-3.6-flash`, not the floating `-latest` alias). Google renames and retires models on its own schedule, and a rename should be a config edit, not a code change. Because the judge decides ratings, a model that changes silently is worse than one that stops loudly.

## 2026-09-18 — oriui pinned at exactly `1.0.0-rc.18`; we keep our own outline token

- **Exact pins on all three packages, in lockstep.** A caret range on a prerelease would start matching a future stable `1.0.0`, which we'd then ship without deciding to. The `rc` dist-tag moves.
- **We don't adopt `--ori-color-outline`.** It is a 12% tint of `currentcolor`, a hairline that follows text colour. `--jp-color-outline` is a fixed per-theme colour that `scripts/check-contrast.mjs` holds to the 3:1 non-text bar, which a tint can't meet. The two tokens share a name but do different jobs.
- **A dependency bump is verified in a rendered browser, not in a diff.** A stale Vite pre-bundle kept serving the old build after the install, so a new prop showed up as a literal attribute (`docs/NOTES.md`).

## 2026-09-18 — One auth gate: any action can raise the sign-in modal and then resume

Without a shared gate, each view improvises its own answer to "this needs a session" (a toast pointing at the menu, a dead-end error phase, a panel), and none of them resumes what the visitor was doing. One small store answers for all of them.

- **`useAuthGate` holds the intent; `AuthDialog`, mounted once in `App.vue`, renders it.** `ensure(reason)` resolves `true` at once for a signed-in visitor. Otherwise it opens one modal, and the caller resumes the moment the visitor signs in, so the requested action actually happens.
- **The transport forgets a dead session; only the caller asks for a new one.** `http.ts` sees every 401, so a `setUnauthorizedHandler` hook clears the store there. Asking is a UI decision: a background poll or a WS reconnect must never raise a modal unprompted, and only the caller knows whether someone is waiting.
- **The gate awaits the session restore, and the store owns the restore.** It restores once, at construction, and `ready()` returns that one promise. Otherwise a click during the restore would read a signed-in visitor as signed out.
- **Page-level empty states stay inline; interrupted actions get the modal.** The anonymous state of `/leaderboard` is "you can't see this yet", so it stays a panel. Entering a duel on `/play` is an action, so it gets the modal and then starts automatically.
- **The modal takes the keyboard.** Window-level shortcuts return early while it is open. Without this, Ctrl+Z edited the canvas behind the modal and Ctrl+S queued duplicate saves.
- **A 401 mid-action does not replay the action.** Minutes may have passed, the canvas may have changed, and the visitor may sign in as someone else. They get the modal, their canvas is intact, and the button is where they left it.
- **Declining is never a dead end.** Dismissing the modal on `/play` lands on the retry card.
- **Mounted at the app root.** `.shell__overlay` sets `pointer-events: none`, which a `<dialog>` inherits even though it paints in the top layer (`docs/NOTES.md`).

## 2026-09-18 — Rate limiting is per-IP, in-process, three tiers

- **Per-IP, not per-user.** Credential stuffing and registration abuse hit `/api/auth/*` before there is a user to key on. Every tier keys on `ClientIP`.
- **In-process token buckets, not a shared store.** v1 runs a single instance and needs no Redis. The known limitation: with N instances, the effective ceiling is N times the configured one.
- **Three tiers, each with its own limiter.** With a shared bucket, cheap default traffic could drain the auth budget. The policy rows are tried in order: `auth-strict` (bcrypt is expensive), then the write tier (routes that create real work: a render, a judge call, storage), then a generous default that a normal page load never comes close to.
- **A full bucket map fails open.** The limiter caps tracked keys at 100,000 and evicts idle ones. If the map is full, a new key is let through untracked. Refusing it would let an attacker with enough source IPs block every other caller's first request.

## 2026-09-18 — Scoped SFC styles with BEM names, not CSS Modules

- **Specificity isn't a fight here.** oriui authors its rules in cascade layers and inside `:where()` (zero specificity). Our unlayered styles beat every oriui layer by cascade alone.
- **We already write BEM**, and readable names next to oriui's own BEM classes read as one system in devtools. Modules hash names by default.
- **Rejected: global BEM with no scoping** (oriui's approach). It suits a library consumed without a build step. In an app, it leaves discipline as the only guard.
- **Given up:** class names as typed JS values. We don't need them, since our styling is per-view chrome.
- **Two footguns:** a scoped rule can't reach into a child component without `:deep()`, and Vue applies the parent's scope id to a child's root element, so a parent rule can style that root by accident.

## 2026-09-18 — The server applies its own migrations at boot

The host has no shell (one-off commands are a paid feature), so "migrate, then roll the binary" is a sequence nobody can perform there. The first deploy came up against an empty database and reported healthy.

- **The binary embeds `server/migrations/` and runs goose before serving** (`AUTO_MIGRATE`, default true). goose is therefore a library dependency as well as a CLI; the `.sql` files didn't move, so local work still uses the CLI. sqlc stays a build-time CLI.
- **Rejected:** a pre-deploy command or one-off job (paid on the host), and migrating from a developer machine each release (every deploy would need a person with production credentials).
- **Trade-off:** a bad migration now fails the deploy. For a single-instance service that is the right side of the trade, because an unmigrated database is broken anyway. A Postgres session advisory lock (`WithSessionLocker`) makes a second instance wait instead of racing the same DDL.
- **`/readyz` verifies the schema exists** (`to_regclass`), not merely that Postgres answers.

## 2026-09-18 — `TRUST_PROXY` defaults to false, and the client IP is the rightmost `X-Forwarded-For` entry

The rate limiter and the request log both need an honest answer to "what is the caller's address".

- **Default false.** A direct caller can put anything in `X-Forwarded-For` and evade a per-IP limit. Until an operator says a proxy sits in front, only `RemoteAddr` is trusted.
- **The rightmost entry, once trusted.** Each hop appends the address it received the connection from, so the leftmost entry is whatever the client claimed. The rightmost entry is what the trusted proxy itself saw, which the client can't forge.
- **Assumes exactly one proxy hop** (the host's load balancer). A CDN in front of it would need the second-from-right entry (`docs/NOTES.md`).

## 2026-09-18 — The Go binary serves the SPA: one origin, one image

- **Rejected: a separate static host plus a proxy and CORS.** The httpOnly SameSite session cookie and the WS same-origin allow-list (`docs/API.md` §9.1) both assume one origin. A second origin would need CORS, credentialed cross-origin requests and a proxy that forwards the WS upgrade.
- **`internal/platform/web.SPA` serves `apps/web`'s `dist/`** (`STATIC_DIR`), registered last on `/` so every API route takes precedence. It serves a real file when one exists (immutable caching under `/assets/`) and `index.html` otherwise, for client-side routes.
- **One image, Go on a Node base.** `RENDER_MODE=node` spawns a local Node child process, which needs `node` and node-canvas's shared libraries next to the server binary.

## 2026-09-18 — Render for hosting, with an external Postgres

- **No Render-managed database.** A free Render Postgres expires after 30 days. `DATABASE_URL` is set by hand in the dashboard against an external free Postgres that doesn't expire.
- **An external uptime pinger against `/readyz`.** A free Render service sleeps after about 15 idle minutes, and Render's own scheduled jobs are paid. We ping `/readyz`, not `/healthz`, because it proves the instance can reach Postgres.
- **The image stays host-agnostic.** Every setting is env-driven, so the same artifact runs on another host or under plain `docker run`.

## 2026-09-18 — Exhausted judging ends as `done` + `resolution = 'aborted'` (no Elo); judging passes are capped

**1. A match could wedge in `judging` forever.** The stuck-judging watchdog re-fires a stale pass only while `judge_attempts` is below the cap (3). After three failures (no `node` on PATH, an OOM kill, a node-canvas fault), nothing moved the match again. Both players had submitted, and the duel was lost.

- **The end state is `done` with a third `resolution`, `'aborted'`:** no winner, no Elo, and a player-facing `judge_reason` saying the round could not be scored. Two alternatives were rejected:
  - A new `errored` status would widen the five-state machine every consumer switches on, for what is just "terminal, no result". That is what `done` + `resolution` already models.
  - Forcing `abandoned` would be false, because `abandoned` means nobody drew.

  `'aborted'` reuses the existing terminal path (`SetMatchResult` → `publisher.Resolved` → `result` frame, `ready: true`).
- **Atomic, and it never overwrites a verdict.** `abortJudging` takes the same `GetMatchForUpdate` row lock as every lifecycle write and rechecks both status and attempt count inside it. It writes through `SetMatchResult`, not `writeFinalResult`, so no score and no rating are applied: the round is closed unscored.
- **The abort sweep is the exact complement of the re-fire sweep.** `ListExhaustedJudgingMatches` and `ListStuckJudgingMatches` use `>=` and `<` against the same cap, with the same stale window and the same partial index, so a wedged row is always in exactly one list. Migration `00005` widens the `resolution` check constraint. It also backfills the `judging_started_at` that `00004` left null, because a null value matched neither query.
- **Ladder.** An aborted match is excluded from `ListTopRatings`, like an abandoned one. The result DTO reports `isTie: false`, because having no verdict isn't a drawn duel.

**2. Rendering was unbounded.** Every judging pass under `RENDER_MODE=node` spawns two node-canvas processes, and a boot drain could dispatch 256 passes at once. On a 512 MB instance that is a fork bomb.

- **A counting semaphore bounds concurrent judging passes** on both dispatch paths (`JUDGE_CONCURRENCY`, default 2, `NewServiceWithConcurrency`).
- **The pass semaphore never blocks.** A pass that can't get a slot stays in `judging`, and the stuck-judging sweep picks it up later. Nothing waits, so shutdown can't deadlock against a render.
- **The sweeper takes its slot before `refireJudging`**, because re-firing increments `judge_attempts`. A retry spent on a pass that never ran would walk a healthy match toward the abort cap.
- **The same number sizes a semaphore inside `NodeRenderer`.** `/api/practice` and `/api/guess` render inline on the request, so a burst in the write tier would otherwise start thirty processes at once. The renderer's bound blocks rather than refuses: every caller already passed a bound of its own, the wait ends with the caller's context, and a slower render beats a lost duel.

## 2026-09-18 — `ENV` is mandatory, with no default

`CookieSecure` derives from `ENV != "dev"`. With a `dev` default, a deploy that forgets `ENV=prod` ships a non-Secure session cookie without complaint. `config.Load` now requires exactly `dev` or `prod` and fails otherwise, like `JWT_SECRET` and `DATABASE_URL`. `ENV` also gates the JWT-secret length floor and the default WS-origin allow-list, so a misspelled value is rejected the same way as a missing one.

## 2026-07-17 — Cross-match ladder write: atomic `rating += delta`, not an absolute `SET`

`users.rating` feeds a leaderboard, so a lost update there fabricates standings. Writing `set rating = $2` from a value read earlier let two different matches that seat the same player, resolving at once, overwrite each other's delta. The match `FOR UPDATE` lock serializes per match, not per user.

- **One atomic query:** `ApplyRatingDelta` (`update users set rating = rating + $delta returning rating`). Under Read Committed, Postgres re-evaluates the row after the write lock, so a second writer adds to the first writer's committed value. `writeFinalResult` writes the rating first and derives `rating_before`/`rating_after` from the `RETURNING` value, so the snapshot always agrees with the ladder. It sorts players by `user_id`, so a rematch pair can't deadlock on the two row locks.
- **Rejected: an advisory lock per user.** The judged path reads ratings outside the resolving transaction. A lock would only help if that read moved inside and Elo were recomputed under it, which is a refactor for no gain over removing the read-modify-write.
- **Accepted:** the delta's size comes from a pre-match snapshot, so two concurrent matches may each size their delta from the same starting rating. That is ordinary Elo rating-period behaviour, and the ladder still nets both deltas exactly (`server/internal/game/rating_db_test.go`).

## 2026-07-12 — Live WS realtime: an in-process actor hub behind a `Publisher` seam

A WS layer pushes match-room state so both players see transitions instantly. Wire protocol: `docs/API.md` §9. Lifecycle: `docs/GAME.md` §9. Gotchas: `docs/NOTES.md`.

- **An actor, not a mutex.** `internal/ws.Hub` is one goroutine servicing register/unregister/publish channels. The rooms map is touched only inside that loop, so there is no lock on the hot path. A stalled client is force-closed on a non-blocking send and never waited on, so one bad socket can't stall the fan-out. This is right for a single process; sharding would only pay off at a scale we aren't at.
- **`game.Publisher` is defined in `game` and implemented by `ws`,** so there is no import cycle. `NopPublisher` is the default, which lets the game and its tests run with no realtime code at all. `main.go` wires the hub with `SetPublisher`.
- **`match_state` and `result` are built per recipient.** `drawingId` depends on the viewer (own vs. opponent, `docs/GAME.md` §4.2), so the hub rebuilds those two frames once per user in the room, through the same viewer-scoped read the REST handlers use. Marshalling once would either leak the opponent's drawing mid-round or strip it from everyone. The other frame types carry nothing viewer-specific and are marshalled once.
- **Postgres stays authoritative.** The hub only fans out snapshots of transitions `internal/game` has already committed, and submit stays an HTTP POST. The REST poll loop is never removed: it drops to a 15s reconciliation cadence while a socket is live and snaps back to 2s on disconnect, so a round completes without the socket.
- **Cookie auth, a `4001` expiry close, strict same-origin.** The handshake reuses the `jp_session` cookie, `RequireAuth` and the membership-hidden 404, all before `Accept`. Nothing re-validates the cookie mid-connection, so the JWT `exp` arms a close with code `4001`, which the client reads as "re-authenticate". `OriginPatterns` is an explicit allow-list, never `*` or `InsecureSkipVerify`. A WS handshake bypasses CORS preflight, so otherwise a cross-site page could ride the victim's cookie.

## 2026-07-11 — Opponent-canvas reveal via a membership-gated endpoint, NOT object storage

The `/play` result screen must show the opponent's drawing, but `GET /api/drawings/{id}` is ownership-scoped and 404s a non-owner. The roadmap proposed an object-storage seam (persist each judged PNG, return a URL). But the opponent's vector document already exists (`match_players.drawing_id`), so the only real blocker was authorization.

- **`GET /api/matches/{id}/players/{userId}/drawing`**, authorized by match membership and gated on the match being `done`, returns that document. The client renders it with the editor's own renderer. One SQL query checks all three gates (viewer is a member, match is done, target is a player), so any miss is the same hidden 404.
- **Why:** no new infrastructure, migration or dependency, small responses, and it fits the existing auth model (cross-user reads go through a membership-authorized route). The canvas shown is a client render of the immutable stored document, not the judge's raster, which was only ever needed for scoring. `judgedImageUrl` stays `null`, and object storage is an optional later idea (feed thumbnails, render offload). When the simplest design suffices, it wins over new infrastructure.

## 2026-07-08 — Excalidraw-inspired shared shell for `/draw` and `/play`

- **We borrow Excalidraw's patterns, not its look:** a warm empty-state card with quick actions, corner discipline, tool hotkey badges and cleaner menu organization, all rendered in oriui and the brand orange. A pixel clone would fight the design system, mean maintaining two visual languages, and edge toward brand mimicry.
- **We kept the right-side slide-in drawer** rather than a top-left dropdown. It is non-modal, so the canvas and an in-progress duel stay live behind it, and it can hold the persistent `/play` profile, rating and match context.
- **We kept the bottom-centre floating toolbar** rather than a top bar. `/play` owns the top band for the prompt banner and round timer, so a top toolbar would force the two modes to diverge.
- **The shared shell is a component, not a convention.** `apps/web/src/components/shell/EditorShell.vue` owns the desk, the Konva mount element (`defineExpose({ canvasEl })`) and named region slots (`#top-left/-center/-right`, `#bottom-left/-center/-right`, `#overlay`, `#drawer`). `DrawView` and `PlayView` both compose it. A `mode: 'draw' | 'play'` prop handles the per-mode differences.

## 2026-07-08 — Shell details: right-side menu, drawing names, canvas backdrop, palette

- **The menu opens from the right and is non-modal:** no backdrop, the canvas stays interactive, and it goes full-screen on phones. Because it is non-modal there is no focus trap and no `aria-modal`, but Esc and focus-return are kept.
- **On `/draw`, signed-out visitors come first:** menu actions first, auth at the bottom. "Copy as text" copies the document JSON; "Copy as image" copies a PNG.
- **A drawing's `name` is metadata, not document format.** It is a `drawings.name` column (64-rune cap, default `'new art'`) that the validators never see.
- **The canvas backdrop is a view preference, not document state.** New documents have `background: null`. The editor paints theme paper or a checkerboard behind them on a view-only layer that is never exported (`Editor.setCanvasBackdrop`, persisted in `localStorage['jp.backdropGrid']`). A transparent document exports as a transparent PNG, and the judge renders on white regardless.
- **Palette:** surfaces `#f0f2f6`/`#191919`, backgrounds `#ffffff`/`#121212`, primary `hsl(20 100% 50%)`/`hsl(20 100% 60%)` with dark ink on primary, because white fails AA on this orange. Outlines hold 3:1 against the surfaces.
- **The layers panel is a dropdown on desktop and a bottom sheet on phones.** On phones the toolbar's style controls collapse into an `OriPopover`, and the shortcuts cheat-sheet is desktop-only.

## 2026-07-08 — Hand tool, and a three-layer accessibility check

- **The hand (pan) tool is a non-stroke tool.** It is a `kind: 'pan'` member of the tool union and has no `buildStroke`, and the editor routes it before the stroke path, so it cannot touch the document or history. It adds single-finger touch pan (touch users previously couldn't pan at all) and a grab cursor. `editor.toDocumentCoords` backs a desktop cursor-coordinate readout.
- **Accessibility is checked in three layers, each catching what the others can't:**
  1. **Token-source contrast lint** (`apps/web/scripts/check-contrast.mjs`, colord, part of `lint:all`/`lint:ci`): checks our design-token pairs statically, with no browser.
  2. **Component axe in happy-dom**, upstream in oriui: covers structure (roles, ARIA, names) but not contrast, because happy-dom doesn't render.
  3. **Browser axe over the rendered app** (`test:a11y`, Playwright + `@axe-core/playwright`): the only layer that catches rendered mis-pairings. It is a separate command, not part of `lint:all`. Known issues are allowlisted per element with `AxeBuilder.exclude()`, never by disabling a rule, so `color-contrast` stays active everywhere else.

  APCA (`apca-w3`) is a possible advisory signal, not a gate.
- **We keep our own palette tokens in `main.css`** rather than adopting oriui's `neutral` skin. Adopting it is an open idea (`docs/IDEAS.md`).

## 2026-07-07 — AI Assist design: text drawing commands

The first AI-in-product feature: the player types a prompt and gets drawing operations back. Contract: `docs/ASSIST.md`, `docs/API.md` §10.

- **An `Op` contract over the command seam.** The endpoint returns a small discriminated union of document operations, never Konva objects: `add_layer` (carrying its own `id`) and `add_stroke`, restricted to `line`/`rect`/`ellipse`/`polygon` with no freehand. The Op schema is part of the dual-validator contract (TS and Go, 1:1). `add_stroke.layerId` must resolve to an existing layer or an earlier `add_layer` in the same batch, in array order, so a forward or dangling reference fails validation. Edit ops come in a later phase.
- **A judge-style seam, `internal/assist`.** An `Assist` interface with `FakeAssist` as the dev/CI default and a real impl selected by `ASSIST_MODE`, behind `POST /api/assist/ops` (auth required). The server proxies the provider call so the API key never reaches the client.
- **Every op is validated server-side, with one retry.** Structured output (a JSON schema) guarantees a parseable response, so the retry, which appends the validator's errors to the prompt, only handles semantic failures. After that the answer is `400 validation_failed`, not `422`, because `docs/API.md` keeps one client path for invalid documents.
- **A minimal document summary, never the full document:** `{ canvas: { width, height }, layers: [{ id, name, strokeCount }] }`. Richer signals (bounding boxes, recent strokes) wait until a real prompt needs them, because the format's `bbox` is optional and never populated.
- **A per-user ceiling ships with the feature**, since the demo is public and every call costs quota: a per-user token bucket (`429 rate_limited` with `Retry-After`) plus a daily per-kind allowance (2026-09-20).
- **Ghost preview, then accept.** Returned ops render as a preview outside history. Accept commits the whole batch as one composite command, so it is undone with one Ctrl+Z. Reject discards it. Phasing: `docs/ASSIST.md` §7.

## 2026-07-04 — UX-first: a polished `/draw` and one shared shell, before the `/play` UI

- **`/draw` stays focused but polished:** proper layout and spacing, the slide-in side menu (auth and profile live there, replacing the top session bar), and correct canvas interaction (no drawing outside the document, immediate eraser feedback).
- **A floating bottom toolbar** (tldraw/FigJam style), chosen over patching a top toolbar.
- **One shell for `/draw` and `/play`.** The game adds its chrome (prompt banner, timer, submit) around the same shell, and the two must not diverge.
- **All common keyboard shortcuts** (tool hotkeys, Ctrl+Z/Y, Ctrl+0/±, Ctrl+S), plus a shortcuts cheat-sheet.
- **Canvas bounds:** layers are clipped to the document rect, and gestures that start outside it are ignored. A stroke started inside may run past the edge but is visually clipped, like a Figma frame.
- **AI inside the product is the planned north-star upgrade** (`docs/IDEAS.md` "AI inside the product"). These features land on `/draw`, the one canvas with no opponent and no clock, which is where assist and "what did I draw?" live.

## 2026-07-03 — The authoritative render worker

The judged raster is rendered off the client from the validated vector document (trust boundary, `docs/GAME.md` §6).

- **The renderer is a seam, like the judge.** `render.Renderer` has an in-process `StubRenderer` (a deterministic 1024² PNG) as the default (`RENDER_MODE=stub`), so the server runs with no Node or canvas present. `RENDER_MODE=node` selects the real worker and requires `RENDER_CLI`, and boot fails fast when it is missing or the mode is unknown.
- **One shared renderer.** The worker reuses `@justpaint/editor`'s `renderToStage`, the same Konva + perfect-freehand path the editor draws with (which is why `FREEHAND_VERSION` is pinned). A Go rasterizer would silently diverge. `renderToStage` is the DOM-free core, and the browser's `renderToPNG` and the worker are thin output layers over it.
- **node-canvas + Konva 10**, with `konva/canvas-backend` imported before any Konva use. node-canvas installs from a prebuild, including on Windows.
- **The worker is esbuild-bundled** (`packages/render/dist/render.mjs`). The workspace packages emit extensionless relative imports, which native Node ESM refuses. Bundling avoids churning the frozen contract package. `canvas` stays external.
- **Spawn-per-render:** `render.NodeRenderer` pipes document JSON on stdin and reads a base64 PNG on stdout. That is right-sized for two renders per match. A resident worker is a later optimization.

## 2026-07-03 — Submit, judging and duel immutability

- **Judging is out-of-band, in process.** Submit returns `202`. The last submit flips the match to `judging` inside the submit transaction, then a goroutine renders both documents, calls the judge, maps the positional winner to a player, applies Elo and commits `judging → done`. A queue isn't needed at this size. A pass that crashes is recovered by the stuck-judging sweep (2026-09-18).
- **Last-submit detection is serialized by a match-row lock** (`SELECT … FOR UPDATE`, `GetMatchForUpdate`). Without it, two simultaneous submits could each see the other as missing, and neither would flip to `judging`.
- **Submitting to a match you're not in is `403`,** the one exception to the hidden 404: it is a known-ownership violation by someone who holds the id. The same non-membership on `GET …/result` answers `404`.
- **Submitted drawings are immutable.** `UpdateDrawing`/`DeleteDrawing` carry `and match_id is null`, and the resulting no-op is classified as `ErrDuelLocked` → `409` (vs. `404` for an absent or foreign row). Otherwise a player could `PUT` a better document over their submission before judging.

## 2026-07-03 — Match creation & matchmaking

- **Open-pool auto-join behind a single "play" button.** In one transaction, `POST /api/matches` joins the oldest waiting `open` match the caller isn't in (`open → drawing`). Failing that, it returns the caller's own open match. Otherwise it creates one, with a random active prompt pinned. There is no lobby or invite UI. `FOR UPDATE SKIP LOCKED` sends two simultaneous joiners to different matches, so a match is never double-seated.
- **No duplicate open matches from one player, best-effort.** A sequential re-tap gets the existing match back. Two truly concurrent creates can still open two matches under Read Committed. That is harmless clutter and never a double seat (the `match_players` primary key guards that). The hard fix is open in `docs/IDEAS.md`.
- **Prompt text is withheld until the match leaves `open`,** so a creator waiting alone can't draw early (`docs/GAME.md` §5).
- **The opponent is shown as `userId` + optional `displayName`, never `login`,** which may be an email.

## 2026-06-19 — Phase 0 contract resolutions

### Square canvas + square judge frame
The game canvas is square, 1080×1080, and the judge renders a square 1024×1024 frame, because letterbox bars would count as judged pixels and skew similarity. The free-draw default stays 1920×1080. Only the game pins square (`docs/GAME.md`).

### Game screen visibility
During a round each player sees only their own canvas. Both are revealed on the result screen.

### Ties are allowed
The judge may return `winner: "tie"`, so `matches.winner_player_id` is nullable. Tie rating rules are in `docs/GAME.md`.

### Auth: login (email or nickname) + password, optional display name
Identity is a single `login` that may be an email or a nickname, plus a password. `display_name` is optional. Exact rules: `docs/API.md`.

### Document size / DoS caps
The binding limit is total input points. The numbers are pinned authoritatively in `docs/API.md` §6.

| Limit | Value | Why |
|---|---|---|
| Request body (`http.MaxBytesReader`) | 8 MB | Outer guard. A maximal legitimate document (~100k points ≈ 2.5–3 MB) sits well under it, so the points cap trips first. |
| Total input points | 100,000 | A frantic 5-minute duel is ~30k points. 100k leaves headroom and still rasterizes sub-second at 1024². |
| Points per stroke | 10,000 | Bounds one pathological stroke. |
| Total strokes | 5,000 | A sanity ceiling; the points cap binds first. |
| Layers | 64 | Far above any hand-editing need. |

## 2026-06-19 — Foundational decisions

### Game is the north star; editor is supporting
The centerpiece is an AI-judged drawing duel. A generic paint app is a commodity, while the judged game is the differentiator. The editor (`/draw`) is a supporting mode. Don't build two products.

### Two modes in one site
`apps/web` serves `/draw` (free) and `/play` (the game). The editor exists for the game anyway, so `/draw` is nearly free and useful. Separate sites would split ops and focus prematurely, and the reusability signal already comes from the `editor` package boundary. Splitting later is reversible if the game earns its own brand.

### Use libraries for canvas; don't hand-write an engine
Rendering is **Konva** + **perfect-freehand**. We own only the document model, serialization and a thin editor wrapper, because a custom render engine isn't the differentiator. **Konva over Fabric:** explicit layers, speed, JSON serialization, `vue-konva`, clean PNG export for the judge.

### Backend rewritten in Go as a modular monolith
One Go service (auth, drawings, game, WS hub) and one Postgres: net/http + pgx + sqlc + goose + golang-jwt + bcrypt + slog + coder/websocket. A monolith fits a solo project; microservices would be over-engineering. Don't refactor the old NestJS — replace it.

### Vector document persisted as jsonb
Drawings are stored as a structured vector document (`Document/Layer/Stroke`), not a PNG. That gives small storage, clean export, real layers, replay, and a clean input for the judge's render. The schema is independent of Konva's internal JSON, to avoid vendor lock-in.

### The ML judge is external
The ML judge is a separate project built by an external collaborator. We define the `Judge` interface (prompt + 2 images → `{scoreA, scoreB, winner, reason}`) and ship a fake, and the real judge integrates over HTTP against that contract. Never block on the ML.

### Greenfield
No production data to preserve, so the schema and format can be redesigned freely. Revisit the migration strategy if that changes.

### Component library: oriui
The frontend uses oriui, a component library we also maintain, published on npm, instead of `vueinjar`. We dogfood it here.
