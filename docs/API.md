# API contract

> **The HTTP surface.** Every route the Go modular monolith exposes: auth, drawings CRUD, the async duel, the live WS realtime layer, AI assist, single-player practice, the ratings leaderboard, and the free-draw AI guess. The single source of truth for the **error envelope**, the **auth cookie**, the **DoS cap numbers**, **pagination**, and the **HTTP status map** — sibling docs reference these rather than re-declaring them. §9 owns the **shipped** WS wire protocol (`feat/ws-realtime`, 2026-07-12); §10 owns the AI-assist HTTP edge (contract owned by `docs/ASSIST.md`); §11 owns the leaderboard read; §12 owns the single-player practice HTTP edge (contract owned by `docs/GAME.md`'s practice section and the `Critic` seam in `docs/JUDGE.md`); §13 owns the `/draw` guess edge (the `Guesser` seam in `docs/JUDGE.md` §8.3).
>
> **Status:** Shipped — every route below is live, §10–13 included (they landed as Phase 4 slices; Phase 4 itself still has open stretch items — `docs/ROADMAP.md`). Companions: `docs/DOCUMENT-FORMAT.md` (the keystone schema + the validation contract API.md applies), `docs/ARCHITECTURE.md` (topology, data model, the Judge seam), `docs/JUDGE.md` (judge contract — owns the result shape), `docs/GAME.md` (match lifecycle, canvas, ratings), `docs/DECISIONS.md` (the "why"). When in doubt those win; this doc does not relitigate them.

## 0. What this doc owns vs. references

API.md is the **sole owner** of: the JSON error envelope, the HTTP status map, the `jp_session` cookie name + flags, the DoS cap numbers, pagination params, the route list, and the **ownership-scoping rule** at the HTTP layer.

It **references, never re-declares**:
- **`drawings` table DDL + the document validation steps** → `DOCUMENT-FORMAT.md` §7 (and §2 logical-coord / 8192 bound, §10 `RenderOptions` + trust boundary). API.md applies that validator; it does not restate the per-field invariants.
- **`users` / `prompts` / `matches` / `match_players`** → `ARCHITECTURE.md` §7 (column source). Match/rating specifics → `GAME.md`.
- **Judge result shape, `winner` type, tie rule, raster size, background** → `JUDGE.md` (single owner). API.md only carries the HTTP edges that trigger judging.

## 1. Conventions

- **Base path.** All routes are under `/api`. JSON in, JSON out (`Content-Type: application/json`) unless noted. Bodies are UTF-8.
- **Auth.** Session is a single JWT access token carried in the **`jp_session` cookie** (§2). Protected routes read it via auth middleware; no `Authorization` header, no token in the body, **never localStorage** (kills the old red flag — `DECISIONS.md` "Auth").
- **Times.** All timestamps are ISO-8601 UTC strings (e.g. `2026-06-19T12:00:00Z`).
- **Ids.** All resource ids are UUID strings.
- **Ownership scope (binding).** Every drawings read/write and every game resource is scoped by the authenticated user. A resource owned by someone else is **hidden as `404 not_found`**, never `403`, so existence does not leak (no IDOR; see §4, §8). The one exception: an explicit ownership violation that the client already knows exists (e.g. submitting to a match you are not a player in) returns `403 forbidden`.
- **Unknown fields** in request bodies are **tolerated, not rejected** for the document payload (forward-compat — `DisallowUnknownFields` is *not* used; `DOCUMENT-FORMAT.md` §7 step 2). Auth/game request bodies decode strictly into their small fixed shapes.

## 2. Auth cookie — `jp_session`

The session cookie is pinned here; `JUDGE.md` / `GAME.md` just say "the `jp_session` cookie".

```
Set-Cookie: jp_session=<jwt>; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=<ttl>
```

- **`HttpOnly`** — JS can't read it (XSS can't exfiltrate the token).
- **`Secure`** — HTTPS only.
- **`SameSite=Lax`** — SPA + API are same-site in v1; Lax is the right default and blocks cross-site CSRF on the cookie. (Revisit to `Strict`/CSRF-token only if a cross-site embed is ever needed.)
- **`Path=/`** — sent to the whole API.
- **`Max-Age`** — the access-token TTL (single token for v1; refresh tokens are a deferred later option — do not spec them now, `DECISIONS.md` / brief §5).
- **JWT secret is MANDATORY.** The server **fails fast at startup if the signing secret is unset** — no empty-string fallback (the old red flag dies here). Tokens are HS256 over a secret from config/env.
- **Logout** clears the cookie by re-issuing it expired (`Max-Age=0`).

The token's claims carry at minimum the user id (`sub`) and expiry (`exp`). Claims are server-trusted; never re-derive identity from the request body.

## 3. Error envelope & status map

**One shape, everywhere.** Every non-2xx response is exactly:

```json
{ "error": { "code": "string_snake_case", "message": "human readable" } }
```

- `code` — a **stable machine string** the client can switch on. Closed v1 set: `validation_failed`, `invalid_credentials`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `document_too_large`, `rate_limited`, `internal`.
- `message` — human-readable, for logs/dev/toasts. It **MUST NOT leak which credential field was wrong** (anti-enumeration, §5) and MUST NOT echo internal detail (stack traces, SQL).

**Status → code mapping (pinned):**

| HTTP | `code` | When |
|---|---|---|
| `400` | `validation_failed` | Malformed JSON, missing/!typed fields, or a **semantically invalid document** (fails any `DOCUMENT-FORMAT.md` §7 invariant). We prefer `400` over `422` for doc-invalid, for one consistent client path. |
| `401` | `unauthorized` | No/expired/invalid `jp_session` on a protected route. |
| `401` | `invalid_credentials` | Login failed — bad login **or** bad password (generic; §5). |
| `403` | `forbidden` | Authenticated but acting on a resource you don't own where existence is already known (e.g. submitting to a match you're not in). |
| `404` | `not_found` | Resource absent **or** owned by someone else (hidden — §1 ownership scope). |
| `409` | `conflict` | Illegal state transition (e.g. submitting to a `done`/`judging` match; double-submit; registering a taken `login`). |
| `413` | `document_too_large` | Request body exceeds the 8 MB cap — tripped by `http.MaxBytesReader` before parse (§6). |
| `429` | `rate_limited` | Throttled (auth endpoints especially). |
| `500` | `internal` | Unexpected server fault. Message is generic; detail goes to `slog`, not the client. |

> `422` is reserved (not used in v1): semantic document errors return `400 validation_failed` for a single client error path.

### 3.1 Rate limiting

Every request passes a **per-client-IP token bucket** before routing; the tier is
chosen by the first matching rule, and each tier has its own independent budget.

| Tier | Applies to | Burst | Sustained |
|---|---|---|---|
| strict | `POST /api/auth/*` | 10 | 1 per 6s |
| write | `POST`/`PUT`/`DELETE` on `/api/matches*`, `/api/drawings*`, `/api/practice`, `/api/guess` | 30 | 1 per 2s |
| default | everything else, including reads, the WS upgrade and the served SPA | 300 | 5 per s |

A throttled request gets **`429 rate_limited`** in the standard envelope plus a
**`Retry-After`** header (seconds). `POST /api/assist/ops` keeps its own
*per-user* bucket on top of this (§10) — that one guards API spend, not abuse.
`POST /api/matches`, `POST /api/practice` (§12), `POST /api/guess` (§13)
**and** `POST /api/assist/ops` (§10) each layer on a third, unrelated
`429 rate_limited`: a **daily AI-call budget** (`GAME.md` §4.3),
per-user-per-kind and per-provider, guarding a resource this per-IP bucket
cannot see at all — a free-tier quota measured in requests per **day**, not
per second, spent alike by a duel entering judging, a practice run being
critiqued, a free-draw canvas being guessed at and an assist prompt reaching
the model. Unlike the tiers above it carries **no `Retry-After`** header: the
window rolls continuously, so there is no fixed reset to name. It is checked
before the expensive work and then **re-tested by the ledger write that records
the call** — one conditional statement that writes the call's rows only while
the caller is under their cap. On `POST /api/practice` and `POST /api/guess`
that write sits *after* the authoritative render, so this `429` can arrive after
it as well as before; on `POST /api/assist/ops` it sits immediately before the
model call. (`POST /api/matches` has no such second gate — a duel's ledger rows
are unconditional, §8.) A per-kind refusal always names the caller's own cap;
the global one still names nothing (`GAME.md` §4.3). (`GET
/api/practice/prompt` is a cheap read and sits in the generous `default` tier,
not `write` — only the scoring `POST` costs a render and a judge call.
`POST /api/guess` is in `write` for the same reason, and the daily budget is
**not** a substitute for that row: under `JUDGE_MODE=fake` guess has no
provider and so is unbudgeted by design, which would otherwise leave the
generous tier as the only thing between an authenticated caller and five
renders a second — each one a `node-canvas` subprocess under
`RENDER_MODE=node`. The daily ceiling guards a provider's quota; this tier
guards our own machine — as does the renderer's own `JUDGE_CONCURRENCY`-sized
semaphore, which bounds how many of those subprocesses exist at once no matter
which route asked for them.)

The client IP is the direct peer unless the server is configured to trust a
proxy (`TRUST_PROXY`), in which case it is taken from the **rightmost**
`X-Forwarded-For` entry — the one hop the proxy itself observed, and therefore the
only one a client cannot forge by sending its own header. Buckets live in
process memory: with several instances behind a load balancer the effective
ceiling multiplies by the instance count.

## 4. Auth routes

All under `/api/auth`. Identity is a single **`login`** credential — an **email OR a nickname** — plus a password. `display_name` is optional. Passwords are **bcrypt-hashed, never plaintext** (`users` columns: `ARCHITECTURE.md` §7).

### `POST /api/auth/register`
Create an account and start a session. **Auth: none.**

Request:
```json
{ "login": "ada@example.com", "password": "correct horse battery staple", "displayName": "Ada" }
```
- `login` — required, 3–254 chars, case-folded (stored in `users.login citext`, unique). May be an email or a nickname — we do **not** branch on shape; it's one opaque credential.
- `password` — required, 8–256 chars (length bounds only; no composition rules in v1).
- `displayName` — optional, 1–64 chars. Absent ⇒ stored `null`.

Success `201 Created` — sets the `jp_session` cookie, returns the current user (§4 "current-user" shape):
```json
{ "user": { "id": "…", "login": "ada@example.com", "displayName": "Ada", "rating": 1200, "createdAt": "…" } }
```

Errors: `400 validation_failed` (bad/missing fields); `409 conflict` (login already taken — this is an unavoidable existence signal at registration, accepted because the user is choosing their own identifier); `429 rate_limited`.

### `POST /api/auth/login`
Authenticate and start a session. **Auth: none.**

Request:
```json
{ "login": "ada@example.com", "password": "…" }
```

Success `200 OK` — sets the `jp_session` cookie, returns `{ "user": { … } }` (same shape as register).

Errors: **`401 invalid_credentials`** for *both* unknown `login` and wrong password — **identical response, no timing/shape tell** (anti-enumeration, §5). `400 validation_failed` for malformed body; `429 rate_limited`.

### `POST /api/auth/logout`
End the session. **Auth: required** (no-op-safe if already anonymous).

Request: empty body. Success `204 No Content` — re-issues `jp_session` expired (`Max-Age=0`).

### `GET /api/auth/me`
Current user (session probe used by the SPA on load). **Auth: required.**

Success `200 OK`:
```json
{ "user": { "id": "…", "login": "ada@example.com", "displayName": "Ada", "rating": 1200, "createdAt": "…" } }
```
Errors: `401 unauthorized` if no valid session. (`password_hash` is **never** serialized in any response.)

## 5. Anti-enumeration (auth)

- **Login** returns a single generic `401 invalid_credentials` for *both* "no such login" and "wrong password" — same `code`, same `message`, same status. The handler runs a bcrypt comparison even on unknown-login (against a dummy hash) so response timing does not distinguish the two cases.
- **Register** necessarily reveals a taken `login` via `409 conflict` (the user picks their own identifier) — this is the deliberate, accepted exception.
- No endpoint confirms whether a given email/nickname exists outside these two paths.

## 6. DoS caps (binding for the Go validator)

These exact numbers are pinned **here** (`DOCUMENT-FORMAT.md` §7 step 1 & step 5 and `DECISIONS.md` "Document size / DoS caps" defer the numbers to API.md). They apply to every drawings write path (create + update), to game submit (which writes a drawing), and to practice scoring (§12) — which runs the identical validator over the identical document but, unlike a submit, never persists it.

| Limit | Value | Enforced by | Notes |
|---|---|---|---|
| Request body (`http.MaxBytesReader`) | **8 MB** | Outer guard, **before** JSON parse | A max-legit doc (~100k points ≈ 2.5–3 MB jsonb) sits well under it, so the *points* cap trips first on real drawings. Over-cap ⇒ `413 document_too_large`. |
| **Total input points (all strokes)** | **100,000** | Document validator | **THE binding semantic cap.** Sum of every point across every stroke on every layer. |
| Points per single stroke | **10,000** | Document validator | Bounds one pathological stroke. |
| Total strokes (all layers) | **5,000** | Document validator | Sanity ceiling; the points cap binds first. |
| Layers | **64** | Document validator | Far above any hand-editor need. |
| Drawing `name` | **64 runes** | Drawings handler | Metadata, **not** part of the document (the validators never see it). Counted in runes like the layer-name cap; over-cap ⇒ `400 validation_failed`. |

- The **8 MB body cap** is wired via `http.MaxBytesReader` on the request body of every document-bearing route; it trips *before* allocation/parse, so a multi-million-point blob can't OOM the parser or the render worker.
- We **do not advertise a per-layer stroke cap** — the total-points budget makes it unreachable (`DOCUMENT-FORMAT.md` §7 step 5).
- A document failing any cap is **rejected**, not truncated: `413 document_too_large` for the byte cap, `400 validation_failed` (with a specific `message`) for the semantic caps.

## 7. Drawings CRUD

All under `/api/drawings`. **Auth: required** on every route. **Every operation is ownership-scoped by the authenticated user** (`owner_id`) — no IDOR; foreign-owned ids are `404 not_found` (§1). The `drawings` table DDL is owned by `DOCUMENT-FORMAT.md` §7 (columns: `id, owner_id, match_id, name, doc_version, width, height, document jsonb, thumbnail_url, created_at, updated_at`).

**The write path (create + update) runs the Go document validator at the write edge, exactly per `DOCUMENT-FORMAT.md` §7 (steps 1–6):** 8 MB `http.MaxBytesReader` → typed decode via `Stroke.UnmarshalJSON` (allow unknown fields) → nullable `*Color` tri-state → invariants (known `version`; `1 ≤ width,height ≤ 8192`; ≥1 layer; hex-color regex; enum membership; `opacity`/`pressure ∈ [0,1]`; finite numbers; sizes/radii/tapers ≥0; `rx,ry>0`; rect `width,height>0`; **`strokeWidth>0` whenever `stroke` present**; point arity freehand-3-tuple≥1 / line-2-tuple≥2 / polygon-2-tuple≥3; `id` non-empty ≤64, unique across all layers+strokes) → DoS caps (§6) → server **derives** `doc_version`, `width`, `height` columns from the validated doc. The client does **not** set those columns; any client-sent values are ignored.

The request payload is `{ "document": <vector document>, "name"?: <string> }`. The `document` value is the full schema from `DOCUMENT-FORMAT.md` §3–§5. `name` is optional user-editable **drawing metadata** — it lives outside the document and never reaches the document validators. It is trimmed of surrounding whitespace and capped at **64 runes** (§6); absent/blank means "no name sent" (see each route for the effect). A client `thumbnail` may ride along but is **advisory only** (trust boundary, `DOCUMENT-FORMAT.md` §10) and is never the source of truth.

### `POST /api/drawings`
Create a free-draw drawing. `match_id` is **null** (duel submissions are created via the game submit route, §8.3, not here).

Request:
```json
{ "name": "sunset study", "document": { "version": 1, "width": 1920, "height": 1080, "background": "#ffffff", "layers": [ … ] } }
```
- `name` — optional, ≤ 64 runes after trimming (§6). Absent/blank ⇒ the drawing is created as **`"new art"`** (the DB default).

Success `201 Created`:
```json
{
  "drawing": {
    "id": "…", "ownerId": "…", "matchId": null,
    "name": "sunset study",
    "docVersion": 1, "width": 1920, "height": 1080,
    "thumbnailUrl": null,
    "createdAt": "…", "updatedAt": "…"
  }
}
```
> The metadata envelope does **not** echo the full `document` by default (it can be large). Fetch the body via `GET /api/drawings/{id}`.

Errors: `400 validation_failed`, `413 document_too_large`, `401 unauthorized`, `429 rate_limited`.

### `GET /api/drawings/{id}`
Fetch one drawing **including its document body**. **Ownership-scoped** — foreign-owned ⇒ `404`.

Success `200 OK`:
```json
{
  "drawing": {
    "id": "…", "ownerId": "…", "matchId": null,
    "name": "sunset study",
    "docVersion": 1, "width": 1920, "height": 1080,
    "document": { "version": 1, "width": 1920, "height": 1080, "background": "#ffffff", "layers": [ … ] },
    "thumbnailUrl": "https://…/thumb.png",
    "createdAt": "…", "updatedAt": "…"
  }
}
```
Errors: `404 not_found`, `401 unauthorized`.

### `GET /api/drawings`
List the caller's drawings, **newest first**, **paginated**. Metadata only — **no `document` body** in list items (keeps the list cheap).

Query params (pagination — pinned, cursor-based):
- `limit` — page size, default **20**, max **100**. Out-of-range ⇒ clamped.
- `cursor` — opaque cursor from a previous page's `nextCursor`; absent ⇒ first page. (Encodes `(created_at, id)` for a stable keyset; do not hand-craft it.)
- `kind` — optional filter: `free` (`match_id IS NULL`) | `duel` (`match_id IS NOT NULL`) | `all` (default). (Named `kind`, not `mode`, to keep `mode` reserved for the **match transport** field `matches.mode` = `async|live` (§8) — the two are unrelated.)

Success `200 OK`:
```json
{
  "drawings": [
    { "id": "…", "matchId": null, "name": "sunset study", "docVersion": 1, "width": 1920, "height": 1080,
      "thumbnailUrl": "https://…/thumb.png", "createdAt": "…", "updatedAt": "…" }
  ],
  "nextCursor": "eyJ…",   // null when there are no more pages
  "limit": 20
}
```
Errors: `400 validation_failed` (bad cursor / bad `kind`), `401 unauthorized`.

### `PUT /api/drawings/{id}`
Replace a drawing's document. **Ownership-scoped.** Full replace (no partial patch in v1); runs the full validator + caps. The server re-derives `doc_version/width/height`. `match_id` is immutable here (a free save stays free; you cannot retarget it at a match).

Request: `{ "document": { … }, "name"?: "…" }` (same as create).
- `name` present ⇒ **replaces** the stored name (same trim + 64-rune cap). Absent/blank ⇒ the existing name is **kept** — an update is a document replace, not a rename, so a client that only re-sends the document never clobbers the name. (Implemented as `COALESCE` in the update SQL, not a read-modify-write.)

Success `200 OK` — returns the updated metadata envelope (same shape as `POST`).

Errors: `400 validation_failed`, `413 document_too_large`, `404 not_found`, `401 unauthorized`, `409 conflict` (if the drawing is a locked duel submission, §8 — a submitted duel drawing is immutable).

### `DELETE /api/drawings/{id}`
Delete a drawing. **Ownership-scoped.**

Success `204 No Content`.

Errors: `404 not_found`, `401 unauthorized`, `409 conflict` (a drawing already submitted to a match cannot be deleted while the match is live — it is referenced by `match_players.drawing_id`).

## 8. Game — the async duel

All under `/api/matches`. **Auth: required.** the async duel is **HTTP-complete** (`mode: "async"`); a live WS push layer (§9) rides on top as a latency upgrade, not a replacement. The full lifecycle, state machine, canvas size (1080×1080), visibility rules, and ratings live in **`GAME.md`**; the judge result shape lives in **`JUDGE.md`**. API.md carries only the HTTP edges.

**Lifecycle recap** (owned by `GAME.md`): **create match (pins ONE prompt for both players) → both players draw the SAME prompt independently → submit (server renders the AUTHORITATIVE judged raster from the vector document, off the player's machine — never a client PNG) → judge → result.** Match states (`matches.status`, `ARCHITECTURE.md` §7): **`open | drawing | judging | done | abandoned`**. Ties are allowed (`matches.winner_player_id` nullable).

During a round each player sees **only their own canvas**; both are revealed on the **result** (§8.4) — so `GET /api/matches/{id}` redacts the opponent's drawing until the match is `done`.

### `POST /api/matches`
Create (or auto-join) an async match. The server pins **one prompt** for both players. **Auth: required.**

Request (all optional):
```json
{ "mode": "async" }
```
- `mode` — `"async"` only in v1 (`"live"` is later, §9). Absent ⇒ `"async"`.
- (Matchmaking — how the second player is paired, open-match pool vs. invite — is a `GAME.md` concern. v1 may create an `open` match the next caller joins, or pair immediately; the route shape is stable either way.)

Success `201 Created` (the caller opened a new match and is waiting — or was returned their existing open one):
```json
{
  "match": {
    "id": "…", "mode": "async", "status": "open",
    "prompt": { "id": "…", "text": null },
    "canvas": { "width": 1080, "height": 1080 },
    "players": [ { "userId": "…", "displayName": "Ada", "submitted": false } ],
    "drawingDeadline": null,
    "serverTime": "2026-07-11T12:00:00.000000000Z",
    "createdAt": "…", "updatedAt": "…"
  }
}
```
> **Prompt text is `null` while `status` is `open`.** The `text` is redacted until the match enters `drawing`, so a creator waiting alone cannot pre-draw before the opponent joins (reveal timing owned by `GAME.md` §5). When this same `POST` **auto-joins** a waiting match instead of opening one, the response is `status: "drawing"` with `text` populated and both players listed.
> `canvas` echoes the canonical **1080×1080** game canvas (owned by `GAME.md`) so the client configures the editor without guessing. The submitted document's `width`/`height` MUST match it (enforced at submit, §8.3).
> `drawingDeadline` is `null` while `status: "open"`; once the roster fills and the match flips to `drawing` it becomes an absolute RFC3339Nano UTC instant (`now() + 90s`, the server's clock — `GAME.md` §4.1). `serverTime` is the response-build instant, always present, in the same format, so the client reconciles clock skew instead of trusting its own clock for the countdown.

Errors: `400 validation_failed` (bad `mode`), `401 unauthorized`, `429 rate_limited` — two distinct causes, same code and status, checked in order: the caller's own daily **duel** allowance is spent (message `"you have used all 20 of your duels for today — new ones unlock as the day rolls over"` — the number is the caller's own configured `duel` cap, 20 by default), or — only once they've cleared that check — the whole daily budget of the provider backing the judge is spent (message `"the AI budget for today is spent — this feature resumes tomorrow"`). The **global** refusal discloses nothing about the budget's size; every per-kind refusal names the caller's own cap, from one template with the kind's noun substituted in (`GAME.md` §4.3). Unlike §3.1's tiers, neither carries a `Retry-After` header. This is also the **only** place the budget can refuse a duel: the ledger rows a duel writes — two player rows when the round starts, one provider row per judging pass — are unconditional, because by then the round is under way and refusing would strand it (`GAME.md` §4.3). Full rule: `GAME.md` §4.3; why: `DECISIONS.md` 2026-09-19, 2026-09-20.

### `GET /api/matches/{id}`
Fetch match state. **Auth: required**; caller must be a player ⇒ otherwise `404 not_found` (hidden). **Opponent's drawing is redacted until `status: "done"`** (visibility rule, `GAME.md`).

Success `200 OK`:
```json
{
  "match": {
    "id": "…", "mode": "async", "status": "drawing",
    "prompt": { "id": "…", "text": "a fox riding a bicycle" },
    "canvas": { "width": 1080, "height": 1080 },
    "players": [
      { "userId": "…(me)…",  "displayName": "Ada", "submitted": true,  "drawingId": "…" },
      { "userId": "…(them)…","displayName": "Bo",  "submitted": false }
    ],
    "drawingDeadline": "2026-07-11T12:01:30.000000000Z",
    "serverTime": "2026-07-11T12:01:05.500000000Z",
    "createdAt": "…", "updatedAt": "…"
  }
}
```
- A player's own `drawingId` is visible once they've submitted; the opponent's `drawingId` (and any rendered raster) appears only on the `done` result (§8.4).
- `drawingDeadline` is `null` only while `status: "open"`; `serverTime` is always present (same clock-skew-correction pair as the create response above).
- Errors: `404 not_found` (not a player / no such match), `401 unauthorized`.

### `POST /api/matches/{id}/submit`
Submit the caller's drawing for this match. **Auth: required**; caller must be a player in the match ⇒ otherwise `403 forbidden`. The body is a **vector document** — never a scored PNG (trust boundary, `DOCUMENT-FORMAT.md` §10). The same **8 MB body cap + full document validator + DoS caps** as drawings CRUD apply (§6, §7). The round has a **server-authoritative deadline** (`drawingDeadline`, stamped when the match entered `drawing` — `GAME.md` §4.1); a submit at or after it is rejected (see Errors).

Request:
```json
{ "document": { "version": 1, "width": 1080, "height": 1080, "background": "#ffffff", "layers": [ … ] } }
```
- The server creates the drawing with `match_id = {id}` and `owner_id = caller`, sets `match_players.drawing_id` + `submitted_at`, and locks it (the submitted drawing becomes immutable — see §7 `PUT`/`DELETE` `409`s).
- **The document `width`/`height` MUST equal the match canvas (1080×1080)** ⇒ otherwise `400 validation_failed`.
- A client `thumbnail` may ride along but is advisory only; the **authoritative judged raster is rendered server-side, off the player's machine**, from this document (`GAME.md` / `DOCUMENT-FORMAT.md` §10).

State effects this submit **triggers** (the transitions themselves are owned by `GAME.md` §4.1; the response reflects only what is true at the moment the submission is recorded):
- First submit ⇒ the caller's slot is stamped; match stays `drawing` (still awaiting the opponent).
- Second (final) submit ⇒ the last slot is stamped and the match advances to `judging`. The verdict — render both rasters (1024×1024 judge frame, opaque white background — owned by `JUDGE.md`), call the `Judge`, map the positional `winner` (`"A"|"B"|"tie"`) onto `matches.winner_player_id` (null on tie), update ratings — then drives the separate `judging → done` transition (`GAME.md` §4.1). The submit response does **not** wait for it.

Success `202 Accepted` (the submission is **recorded**; the verdict is produced out-of-band, so the body reflects the post-submit state, never a completed `done`):
```json
{
  "match": {
    "id": "…", "status": "drawing", "you": { "submitted": true, "drawingId": "…" },
    "drawingDeadline": "2026-07-11T12:01:30.000000000Z",
    "serverTime": "2026-07-11T12:00:05.000000000Z"
  }
}
```
> After the final submit `status` is `judging`; before it (opponent still drawing) `status` stays `drawing`. Same `drawingDeadline`/`serverTime` pair as `GET` (above), so the client re-anchors its countdown off this ack without a follow-up `GET`. The client then polls `GET /api/matches/{id}/result` (§8.4) for the verdict once `ready: true` (the WS push in §9 delivers the verdict instantly; polling stays the fallback).

Errors:
- `400 validation_failed` — invalid document or wrong canvas size.
- `413 document_too_large` — over 8 MB.
- `403 forbidden` — caller is not a player in this match.
- `404 not_found` — no such match.
- `409 conflict` — match not in a submittable state (`judging`/`done`/`abandoned`), the caller already submitted (no double-submit), **or the round's `drawingDeadline` has already passed** (message `"round expired"` — the same generic `conflict` code, not a distinct one; the submission is *not* recorded, and the round resolves to forfeit/abandoned as part of rejecting it). The client treats any submit `409` as "go poll the result," not an error toast.
- `401 unauthorized`.

### `GET /api/matches/{id}/result`
The end-of-round result. **Auth: required**; caller must be a player ⇒ `404 not_found` otherwise. **Both canvases are revealed here** (only once `status: "done"`).

If the match is not yet decided, return the in-progress state with `200 OK` (`ready: false`, no winner/scores) — `status` echoes the current match state:
```json
{ "result": { "status": "judging", "ready": false } }
```
- `status` here is whatever the match currently is: `open` / `drawing` / `judging` while still in flight, or **`abandoned`** for a terminated match. **An `abandoned` match returns `200 OK` with `{ "result": { "status": "abandoned", "ready": false } }` and no winner/scores/reason** — there is no verdict to reveal. All five match states are thus accounted for at this endpoint: only `done` is `ready: true`.

When decided (`status: "done"`), `200 OK`:
```json
{
  "result": {
    "status": "done",
    "ready": true,
    "prompt": { "id": "…", "text": "a fox riding a bicycle" },
    "winnerUserId": "…",          // null on a tie (ties are allowed — DECISIONS / JUDGE.md)
    "isTie": false,
    "reason": "left image matches the prompt more closely",  // matches.judge_reason
    "resolution": "judged",       // or "forfeit" / "aborted" — see below
    "players": [
      { "userId": "…", "displayName": "Ada",
        "drawingId": "…", "score": 0.81,
        "ratingBefore": 1200, "ratingAfter": 1212,
        "judgedImageUrl": null },
      { "userId": "…", "displayName": "Bo",
        "drawingId": "…", "score": 0.64,
        "ratingBefore": 1200, "ratingAfter": 1188,
        "judgedImageUrl": null }
    ]
  }
}
```
- `winnerUserId` is the **resolved player id** (`matches.winner_player_id`) — the `game` module mapped the judge's positional `A`/`B`/`tie` onto it at submit time (`JUDGE.md` / `ARCHITECTURE.md` §5). `null` ⇔ `isTie: true`, **except on `resolution: "aborted"`**, where it is null because no verdict exists at all and `isTie` is `false` (a round nobody scored is not a drawn duel). Branch on `resolution` before reading `isTie`.
- `score` / `reason` come from the judge (`match_players.score`, `matches.judge_reason`). `judgedImageUrl` **stays `null`**: it *would* point at the server-rendered authoritative raster in object storage, but object storage is **deferred** (not built). The reveal shows the opponent's canvas via `GET …/players/{userId}/drawing` (below) + a client render instead, so no raster URL is needed for it; the field is kept for a future feed-thumbnail / render-offload use.
- `resolution` is `"judged"` (the normal path — both players submitted, the judge ran), `"forfeit"` (the round deadline passed with exactly one submitter — that player won by default, full Elo, **no judge ran**, `GAME.md` §4.1/§8), or `"aborted"` (both submitted but judging exhausted its retries and never produced a verdict — `GAME.md` §4.1). On a forfeit, both players' `score` is `null` (no judge similarity was produced — never `0`); the forfeiting player's `drawingId` is `null` if they never submitted. On an **abort** the round is terminal (`status: "done"`, `ready: true`) but carries **no verdict at all**: `winnerUserId` and every player's `score` / `ratingBefore` / `ratingAfter` are `null`, **no rating was applied**, and `reason` is a fixed player-facing line explaining the round could not be scored — the one thing a player ever sees about it. Both `drawingId`s are present, so the reveal can still show both canvases. The client branches its result copy on `resolution`, never on the free-text `reason`. Every completed match has a non-null `resolution` (historical pre-migration rows default to `"judged"`).
- Errors: `404 not_found`, `401 unauthorized`.

### `GET /api/matches/{id}/players/{userId}/drawing`
A match participant's submitted **vector document** — how the reveal shows the **opponent's** canvas. **Auth: required**; the caller must be a co-player of the match **and** the match must be `done`, else `404 not_found`. The 404 is a **uniform hide** — it never says which gate failed, so it leaks neither match membership nor status nor the existence of a submission. `userId` may be the caller's own (a uniform participant-drawing read).

Why a dedicated route: `GET /api/drawings/{id}` (§7) is ownership-scoped and `404`s a non-owner, so it cannot serve the opponent's canvas. Authorization here is **match membership**, gated on the reveal (`done`), so an opponent's drawing is never fetchable mid-duel. **No object storage** — the client renders the returned document with the editor's own renderer (the same one that draws the local canvas), so both reveal sides are uniform.

Success `200 OK` — the raw vector document inline (the same shape §7 stores):
```json
{ "document": { "version": 1, "width": 1080, "height": 1080, "background": "#ffffff", "layers": [ … ] } }
```
- Errors: `404 not_found` (not a co-player, match not `done`, or no such match / player / submission — all indistinguishable), `401 unauthorized`.

### `POST /api/matches/{id}/abandon` *(optional, v1-thin)*
Concede / abandon an unfinished match. **Auth: required**; caller must be a player. Moves the match → `abandoned` (terminal). Useful so an opponent who never submits doesn't strand the match forever. (Forfeit/rating effects are a `GAME.md` detail; the route may ship in Phase 3 rather than day one.)

Success `200 OK` — `{ "match": { "id": "…", "status": "abandoned" } }`.
Errors: `404 not_found`, `403 forbidden`, `409 conflict` (already `done`), `401 unauthorized`.

## 9. Live match-room WS — **SHIPPED** (`feat/ws-realtime`)

> The async duel (§8) still runs correctly on HTTP alone. This section owns the **real, shipped** WS wire protocol — a *delivery upgrade* over §8, not a second backend: same match lifecycle, same DTOs, same visibility rule (`GAME.md` §4.2). **Postgres stays the source of truth**; the client's REST poll loop is never removed, only demoted to a slow fallback while a socket is live (§9.5).

### 9.1 Endpoint & gates — `GET /api/matches/{id}/ws`

- **Transport.** `coder/websocket`, an **in-process hub of match rooms** inside `server/` (`internal/ws`) — same binary, same auth, same Postgres. A single goroutine owns all room state (no mutex); the hub never mediates a mutation, it only fans out snapshots of transitions `internal/game` already committed.
- **Auth: the same `jp_session` cookie as REST**, behind the same `RequireAuth` middleware (a WS handshake can't carry a custom header, so cookie auth is the only mechanism) — no/expired/invalid session ⇒ plain `401` **before** the upgrade.
- **Connection caps, refused as `429 rate_limited`.** Before anything else in the handler — before the membership check's DB round-trip — a process-wide concurrent-connection cap and a per-IP cap (both configured, pre-launch hardening) are checked. Either one being saturated refuses the upgrade with a plain (pre-`Accept`) HTTP `429` and the existing error envelope (`{"error":{"code":"rate_limited", …}}`, §3) — the same status/code REST already uses for throttling, just applied to this route too, not a new code. The per-IP key is the TCP peer (`r.RemoteAddr`), never a client-supplied header. This is **in addition to**, not a replacement for, the per-user-per-match cap below (§9.1 "Per-connection limits").
- **Membership-gated, hidden as 404** — identical to every other match route (§1): a non-player, or a non-existent/non-UUID id, gets a plain `404` before `Accept`, never `403` — the socket never confirms the match exists to a non-member.
- **Strict same-origin.** The handshake's `Origin` is checked against an explicit allow-list (`websocket.AcceptOptions.OriginPatterns`) — **never** `*`, **never** `InsecureSkipVerify`. A WS handshake bypasses CORS preflight, so without this a cross-site page could open a socket riding the victim's auto-attached cookie (the WS analogue of CSRF). The request's own `Host` is always authorized; `WS_ALLOWED_ORIGINS` adds extra hosts only for a split-host deployment (dev's Vite proxy seeds `localhost:*`/`127.0.0.1:*`; prod defaults to empty = same-origin only).
- **Session-expiry close.** Nothing re-validates the cookie mid-connection, so at `Accept` the server reads the JWT `exp` and arms a timer to close the socket with private close code **`4001`** the instant the session would have expired. The client treats `4001` as "re-authenticate," never as a transient drop to reconnect through.
- **Read-idle close.** The server arms a read-idle deadline that resets on every inbound frame; if nothing is heard for `Limits.ReadIdleTimeout`, it closes the socket with private close code **`4002`** — unlike `4001`, a transient condition, so the client may simply reconnect. To keep a healthy-but-quiet connection (long silent stretches while someone draws) from tripping this, the server also proactively probes the peer with a protocol-level ping every `Limits.HeartbeatInterval` (well under the timeout); browsers answer this automatically at the network-stack level, so it requires no client code and isn't affected by background-tab timer throttling. This is independent of, and in addition to, the client's own app-level `{"type":"ping"}` (§9.3), which also resets the deadline when it arrives.
- **Per-connection limits.** The process-wide/per-IP caps above bound a user (or IP) across **all** matches; a separate, pre-existing **per-user-per-match** cap (`internal/ws/hub.go` `wsMaxConnsPerUser`, oldest force-closed on exceed) bounds one authenticated user's socket footprint within a single match room. The two are independent defenses at different scopes, not alternatives.

### 9.2 Server → client frames

Eight frame types, JSON `{ "type": …, … }`. `match_state` / `result` carry the **same DTOs** the REST responses do (`Match` §8.1/§8.2, `MatchResultDone` §8.4) — the hub rebuilds each **per recipient** through the identical viewer-scoped read the REST handlers use, so `GAME.md` §4.2 visibility holds on the wire exactly as over HTTP: a mid-round `match_state` sent to player A carries A's own `drawingId` and never B's. This is a runtime, per-recipient redaction — not a "the frame type has no such field" guarantee.

| `type` | payload | built | fires when |
|---|---|---|---|
| `match_state` | `{ match: <Match> }` | **per viewer** | on connect/reconnect, and on every roster/deadline change |
| `opponent_submitted` | `{ userId }` | shared | any player's submit commits (room-broadcast — including back to the submitter's own other tabs; clients ignore a `userId` that is their own) |
| `judging` | `{}` | shared | the last submit flips the match to `judging` |
| `result` | `{ result: <MatchResultDone> }` | **per viewer** | the match reached `done` — `resolution: "judged"`, `"forfeit"`, **or** `"aborted"` (terminal with no verdict, §8.4) |
| `abandoned` | `{}` | shared | the sweep resolves the match to `abandoned` |
| `opponent_connected` | `{ userId }` | shared | that `userId`'s live-client set goes empty → non-empty (presence) |
| `opponent_disconnected` | `{ userId }` | shared | that `userId`'s live-client set goes non-empty → empty |
| `pong` | `{}` | shared | reply to a client `ping` |

### 9.3 Client → server

Exactly one frame, `{"type":"ping"}` — a heartbeat only. There is no other client→server payload: **the drawing submit still goes over HTTP `POST /matches/{id}/submit`** (§8.3) in every case; the socket never carries document data or any mutation.

### 9.4 Reconnect

On every connect (including a reconnect), the server immediately sends a per-viewer `match_state` snapshot — the same read REST would return to that viewer at that instant. There is **no replay buffer**; a reconnecting client just gets a fresh snapshot, exactly like a fresh poll.

### 9.5 Relationship to the REST poll loop

The socket is additive, never a replacement for §8's poll loop (`GET /matches/{id}` / `GET /matches/{id}/result`). Postgres remains the sole source of truth. The client demotes its poll cadence to a slow (~15s) reconciliation fallback while a socket is open, and snaps back to the fast cadence immediately on any disconnect — so a client that never opens a socket, or whose socket keeps dropping, still completes a correct round over REST alone, just more slowly.

## 10. AI assist — `POST /api/assist/ops`

Turns a natural-language prompt into a validated batch of document operations (**Ops**) the client applies as one composite editor command. **`docs/ASSIST.md` is the contract owner** — the Op schema (§2), the `internal/assist` seam (§3), the doc-summary shape (§4), and the ghost-preview/accept UX (§5) are defined there; this entry carries only the HTTP edge.

### `POST /api/assist/ops`
**Auth: required** (session cookie, like every write route). Rate-limited per user **twice over** — an in-process token bucket on the request *rate*, plus the durable daily AI-call ceiling on the *quota* behind it — because each call can cost real API money (`docs/ASSIST.md` §3.4, `GAME.md` §4.3).

Request:
```json
{
  "prompt": "draw a house with a red roof",
  "docSummary": { "canvas": { "width": 1920, "height": 1080 }, "layers": [ { "id": "…", "name": "Layer 1", "strokeCount": 3 } ] },
  "targetLayerId": "…"        // optional: bias generation onto this layer
}
```
- The full document is **never** sent — only the compact `docSummary` (`docs/ASSIST.md` §4). Request body cap: **~64 KiB** (a prompt plus the minimal summary sits well under it; no document ever rides this route, so it does not share the 8 MB cap in §6).

Success `200 OK`:
```json
{
  "ops": [ /* Op[] — docs/ASSIST.md §2 */ ],
  "note": "Drew the house as a rect body, polygon roof, and two rect windows."  // optional, surfaced in the UI
}
```
- The batch is capped at **`maxOpsPerBatch` = 64** ops and is re-validated server-side (the Go document validator) before the response is sent — the client never receives an unvalidated batch (trust boundary).

Errors:
- `400 validation_failed` — malformed/oversized request body, an empty or over-long prompt, the handler's defense-in-depth re-validation of the impl's output, **or** the model's output still failing validation after the impl's retry budget (`ErrInvalidBatch`; under `ASSIST_MODE=gemini` that is one try plus one retry carrying the validator's own complaint — `docs/ASSIST.md` §3.3). All fold into the same code/status — never `422` (§3 reserves it unused in v1).
- `401 unauthorized` — no/expired/invalid `jp_session`.
- `429 rate_limited` — **two distinct limits**, same code/status. First, the per-user **token bucket** (`docs/ASSIST.md` §3.4), checked before the body is even decoded and the only one of the two that carries a **`Retry-After`** header (seconds). Then the **daily AI-call budget** (`GAME.md` §4.3), checked last of the guards and immediately before the model call — itself two causes in order: the caller's own `assist` allowance is spent (message *"you have used all 40 of your AI drawing requests for today — new ones unlock as the day rolls over"*; the number is the caller's own configured `assist` cap, 40 by default), or, only once they've cleared that, the whole daily budget of the provider behind assist is spent (message *"the AI budget for today is spent — this feature resumes tomorrow"*). The per-kind refusal can come from **either** the check or the ledger write that records the call a line later — the write re-tests the same cap and returns the same error, so the two are indistinguishable on the wire, by design. Neither budget refusal carries a `Retry-After`: the window rolls continuously, so there is no fixed reset to name (§3.1). The budget half fires only when the impl really calls a provider: **`ASSIST_MODE=gemini` is budgeted** (provider `google:<model>`, since 2026-09-20), while `fake` calls nobody and is never billed, so on that mode only the token bucket can produce this status (`GAME.md` §4.3, `ASSIST.md` §3.4). A **third** cause joins them under `gemini`: the provider's *own* quota running out (`judge.ErrQuotaExhausted`) answers with the same global refusal rather than a `500`, exactly as on `/api/practice` and `/api/guess` (`JUDGE.md` §8.1).

## 11. Ratings — the leaderboard

The global rating ladder: the top-rated players with their win/loss records. **`docs/GAME.md` §8 owns the eligibility, tie-break, and ranking rules** (this entry carries the HTTP edge). Read-only — no write route mutates the leaderboard; ratings move only through match resolution (§8).

### `GET /api/leaderboard`
**Auth: required** (session cookie, like every route). A world-readable ladder to every authed user — but `login` is **never** exposed (it may be an email; only `displayName`, which is user-chosen and nullable, is returned — the same privacy rule as the §8 roster).

Query params:
- `limit` — page size, default **20**, max **100**. Out-of-range / non-numeric / absent ⇒ **clamped** (never a 400).

Success `200 OK`:
```json
{
  "leaderboard": [
    { "rank": 1, "userId": "…", "displayName": "Ada", "rating": 1432, "gamesPlayed": 13, "wins": 10, "losses": 3 },
    { "rank": 2, "userId": "…", "displayName": null,  "rating": 1300, "gamesPlayed": 4,  "wins": 1,  "losses": 3 }
  ],
  "limit": 20
}
```
- `rank` is the **1-based row number** over the rating-ordered result (rating desc, then `userId` asc as a stable tie-break — see `docs/GAME.md` §8). `displayName` is nullable. `wins`/`losses` derive from the match winner; a tie is neither, so `ties = gamesPlayed − wins − losses` (not a returned field). Players with **zero finished matches are hidden** (`docs/GAME.md` §8).
- **Pagination deviates from §7 deliberately:** this is **top-N by `limit` only** — no keyset `cursor`, no `nextCursor`. `rank` is an *absolute position* in the global ordering, which a keyset (a relative seek from an opaque row) cannot carry; a cursor page would return rows whose rank the server can no longer state. Deep paging past the top N is out of scope for v1.

Errors:
- `401 unauthorized` — no/expired/invalid `jp_session`. This is the **only** client error: `limit` is clamped rather than rejected, so there is no `400` path.

## 12. Practice — single-player scoring

All under `/api/practice`. **Auth: required** on both routes — a run is recorded against a user and spends their own `practice` share of the daily AI-call budget (§3.1, `GAME.md` §4.3), so there is no anonymous path. Practice is the async duel's (§8) solo sibling: one player, one prompt, the SAME real judge infrastructure and the SAME 1080×1080 canvas — but no match, no roster, no deadline, and the verdict comes back **synchronously**, in the response that submits the drawing. Full rules (why it isn't a `matches` row, why it isn't rated): `docs/GAME.md`'s practice section. The scoring seam (`Critic`, explicitly not the `Judge` contract): `docs/JUDGE.md` §8.2. The "why": `docs/DECISIONS.md` 2026-09-20.

### `GET /api/practice/prompt`
Fetch one prompt to draw. **Auth: required.**

Unlike a duel's prompt — redacted until an opponent joins (§8) — the text is **never redacted** here: there is nobody to pre-draw against. This is a cheap read and sits in the generous **default** rate-limit tier (§3.1), not `write`.

Success `200 OK`:
```json
{ "prompt": { "id": "…", "text": "a jellyfish disco party" } }
```

Errors: `401 unauthorized`; `500 internal` — practice has no critic configured for this deployment (see the `POST` errors below; the same cause, refused before a player spends any time drawing for a prompt that could never be scored).

### `POST /api/practice`
Score one drawing against a prompt. **Auth: required.** Sits in the **write** rate-limit tier (§3.1) — a run costs a render and a judge call, the same work a duel submission costs, from one player instead of two.

Request:
```json
{ "promptId": "…", "document": { "version": 1, "width": 1080, "height": 1080, "background": "#ffffff", "layers": [ … ] } }
```
- The same **8 MB body cap + full document validator + DoS caps** as drawings/duel submissions apply (§6), including the **1080×1080 canvas check** (`GAME.md` §2) — practice draws on the identical judged canvas a duel does. Unknown fields are tolerated, exactly as on the duel submit path (§1). A client `thumbnail` may ride along but is advisory only and is ignored server-side (trust boundary, `DOCUMENT-FORMAT.md` §10).
- **Unlike a duel submission, the document is never persisted** — no `drawings` row is created. It is validated, rendered to the authoritative raster, handed to the critic, and discarded; only the verdict (`score`/`feedback`) survives, in `practice_runs`. A client's own advisory thumbnail is therefore the only picture of a run that outlives its response.

Success `200 OK` — the verdict, produced **synchronously** inside this request (the authoritative render and the critic's call both happen before the response is written, unlike a duel's out-of-band judging, §8.3):
```json
{
  "run": {
    "id": "…",
    "score": 0.0,
    "feedback": "You drew a clean sun symbol, but it does not show any part of the prompt. To depict a jellyfish disco party, try drawing umbrella-shaped jellyfish with wavy tentacles dancing beneath a shiny disco ball.",
    "prompt": { "id": "…", "text": "a jellyfish disco party" }
  }
}
```
- `score` is the critic's similarity-to-prompt reading in `[0, 1]` — the **same scale** a duel's `scoreA`/`scoreB` use (`JUDGE.md` §8.2), so a practice score and a duel score mean the same thing to a player. `feedback` is plain text, ≤500 characters, addressed to the player as "you". `run.id` identifies the `practice_runs` row; there is no `drawingId` in this response (see above — nothing was stored to point at).

Errors:
- `400 validation_failed` — invalid document, or the wrong canvas size.
- `413 document_too_large` — over 8 MB.
- `404 not_found` — `promptId` is not a valid UUID, or names no *active* prompt. A retired prompt answers exactly like a made-up one (§1 ownership-hiding, applied here to "does this prompt still exist" rather than ownership) — deactivation is not detectable by trying to draw for it.
- `429 rate_limited` — the **same daily AI-call budget** a duel draws on (`GAME.md` §4.3), two distinct causes, same code/status, checked in order: the caller's own daily **practice** allowance is spent (message *"you have used all 20 of your scored drawings for today — new ones unlock as the day rolls over"*; the number is the caller's own configured `practice` cap, 20 by default), or — only once they've cleared that check — the whole daily budget of the provider behind the critic is spent (message *"the AI budget for today is spent — this feature resumes tomorrow"*). The **global** refusal discloses nothing about the budget's size; every per-kind refusal names the caller's own cap, from one template with the kind's noun in it. The per-kind refusal has **two** origins that look identical on the wire: the check before the run, and the ledger write between the render and the critic, which re-tests the cap as it stands at that instant — so this `429` can arrive after the authoritative render as well as before it. A third route to the same `429` is the **provider's own quota** running out ahead of ours: the critic's `ErrQuotaExhausted` is answered with the global refusal above rather than a `500`, since a retry cannot succeed until the provider's window rolls (the cause is logged at error level, where it is an operator's problem). Neither carries a `Retry-After` header (same posture as `POST /api/matches`, §8).
- `500 internal` — **practice is not configured on this server.** Under `JUDGE_MODE=http` there is no `Critic` (the collaborator's service has no critique endpoint and was never asked to build one — `JUDGE.md` §8.2), so every call refuses rather than silently scoring with the fake critic. This is **one of the two `500`s in this API whose message names its cause** (the other is the guess route's, §13, refusing for the same reason) — *"practice is not available on this server: the configured judge cannot score a single drawing"* — instead of the usual opaque `"internal error"` (§3): a misconfigured `JUDGE_MODE` is a deployment fact worth surfacing, not a secret, and an opaque message here would let the mistake go unnoticed for a long time.
- `401 unauthorized`.

## 13. Guess — "what did I draw?" on `/draw`

One route, `POST /api/guess`. **Auth: required** — a call spends the caller's own `guess` share of the daily AI-call budget (§3.1, `GAME.md` §4.3), so there is no anonymous path. This is the free editor's AI question and the mirror of assist (§10): assist draws what you say, this says what you drew. It borrows practice's (§12) *shape* — one document up, one verdict back synchronously, nothing stored — but not its question: there is **no prompt**, because nobody supplied an answer, so there is nothing to score against and **no score comes back**, only a label. The seam it runs on (`Guesser` — ours, explicitly not the frozen `Judge` contract): `docs/JUDGE.md` §8.3. The "why": `docs/DECISIONS.md` 2026-09-20.

### `POST /api/guess`
Say what the caller's free-draw canvas is. **Auth: required.** Answers `200`, never `202`: there is nobody to wait for, so the request that asks is the request that learns — the authoritative render and the model call both happen inside it, bounded by `guess.RunBudget` (25s, the same margin practice takes under the server's 30s write timeout, and for the same reason).

Request:
```json
{ "document": { "version": 1, "width": 1280, "height": 720, "background": "#ffffff", "layers": [ … ] } }
```
- The same **8 MB body cap + full document validator + DoS caps** as `POST /api/drawings` and `POST /api/practice` (§6) — the same artefact, so the same ceiling and the same `413 document_too_large` code. Unknown fields are tolerated, exactly as on every other document-bearing route (§1). A client `thumbnail` may ride along but is advisory only and is ignored server-side (trust boundary, `DOCUMENT-FORMAT.md` §10).
- **The canvas may be any valid size.** This route runs `document.ParseAndValidate`, **not** `game.ValidateSubmission` — the duel's extra **1080×1080** rule (`GAME.md` §2) belongs to the duel, where two players are compared and must therefore draw on the same canvas. A free-draw canvas is whatever the player made it, anywhere inside the format's 8192 bound, and importing the duel's rule would `400` exactly the drawings this feature exists to look at. Verified live: a real `/draw` canvas came back **1280×720**, which the duel's square check would have rejected outright.
- **Nothing is persisted.** There is no `guesses` table and no row anywhere: no `drawings` row, nothing to read back, no `id` in the response. The drawing on `/draw` may never be saved at all, so there is frequently nothing for a guess to hang off — and a guess is a moment rather than a record, scoring nothing and gating nothing. The one durable trace is the AI-call ledger row (`GAME.md` §4.3), which records *that a provider call was made*, never what it said.

Success `200 OK`:
```json
{
  "guess": {
    "label": "a cat wearing a hat",
    "confidence": 0.82,
    "alternatives": ["a rabbit in a basket"]
  }
}
```
- `label` is a short noun phrase, **≤80 characters** — deliberately a sixth of a critique's feedback cap, because the shortness *is* the product; the model is asked for ≤70 and the remaining ten are the clamp margin. Writing may itself **be** the drawing: for a carefully drawn word, *"the word HELLO"* is the correct answer.
- `confidence` is in `[0,1]` and means something **different from a score**, despite sharing the scale: a practice/duel score says how well a drawing matched a prompt **we** handed the player, a confidence says how sure the model is of a label **nobody** handed anyone. The two are not comparable and must never be shown side by side as if they were — the `/draw` card renders it as a phrase, never as the number.
- `alternatives` holds **0–2** runner-up guesses, genuinely different subjects rather than rewordings of `label`. It is **always an array on the wire, never `null`** — an absent list and an empty one are the same fact, and a clear drawing having no runner-up is the expected case, not a degraded one.

Errors:
- `400 validation_failed` — invalid document. **Not** a wrong canvas size: there is no size rule here beyond the format's own (above).
- `413 document_too_large` — over 8 MB, tripped by `http.MaxBytesReader` before parse (§6); the same cap and the same code as `POST /api/drawings` and `POST /api/practice`.
- `429 rate_limited` — the **same daily AI-call budget** a duel and a practice run draw on (`GAME.md` §4.3), two distinct causes, same code/status, checked in order: the caller's own daily **guess** allowance is spent (message *"you have used all 2 of your AI guesses for today — new ones unlock as the day rolls over"*; the number is the caller's own configured `guess` cap, 2 by default), or — only once they've cleared that check — the whole daily budget of the provider behind the guesser is spent (message *"the AI budget for today is spent — this feature resumes tomorrow"*). **Every** per-kind refusal names its own cap, from one template with the kind's noun substituted in — the player's cap is the player's to know, and this route is simply where that reads most sharply, because the cap is 2 (`GAME.md` §4.3); the **global** refusal still discloses nothing about the budget's size. The per-kind refusal has two origins that look identical on the wire — the check before the render, and the ledger write between the render and the model call, which re-tests the cap as it stands at that instant — which matters most here, where the cap is two. A third route to the same `429` is the **provider's own quota** running out ahead of ours: the guesser's `ErrQuotaExhausted` is answered with the global refusal rather than a `500`, since a retry cannot succeed until the provider's window rolls (the cause is logged at error level). Neither carries a `Retry-After` header (same posture as `POST /api/matches`, §8).
- `500 internal` — **the guess is not configured on this server.** Under `JUDGE_MODE=http` there is no `Guesser`: the collaborator's service answers a comparative two-image question and has no endpoint that looks at one drawing and names it (`JUDGE.md` §2 is frozen, §8.3). Like practice's, this `500` **names its cause** — *"the AI guess is not available on this server: the configured judge cannot look at a single drawing"* — rather than falling back to `FakeGuesser`, whose "guess" reads ink coverage and has never looked at a picture: a wrong guess is *funny*, so a fabricated one reads as the feature working rather than as the feature being off, which makes it a more convincing lie than a fake score would be.
- `500 internal` (opaque) — the render failed, the rendered raster came back over the **4 MiB** `maxRasterBytes` guard (a server-side fault — the document already passed validation), or the guesser answered with something that is not a valid `Guess`. All three get the usual `"internal error"`.
- `401 unauthorized`.
