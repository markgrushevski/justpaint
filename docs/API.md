# API contract

> **The HTTP surface.** Every route the Go modular monolith exposes for v1: auth, drawings CRUD, and the async duel. The single source of truth for the **error envelope**, the **auth cookie**, the **DoS cap numbers**, **pagination**, and the **HTTP status map** — sibling docs reference these rather than re-declaring them. A forward-looking WS sketch closes the doc, clearly marked **not-v1**.
>
> **Status:** Phase 0 (draft). Companions: `docs/DOCUMENT-FORMAT.md` (the keystone schema + the validation contract API.md applies), `docs/ARCHITECTURE.md` (topology, data model, the Judge seam), `docs/JUDGE.md` (judge contract — owns the result shape), `docs/GAME.md` (match lifecycle, canvas, ratings), `docs/DECISIONS.md` (the "why"). When in doubt those win; this doc does not relitigate them.

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

These exact numbers are pinned **here** (`DOCUMENT-FORMAT.md` §7 step 1 & step 5 and `DECISIONS.md` "Document size / DoS caps" defer the numbers to API.md). They apply to every drawings write path (create + update) — and to game submit, which writes a drawing.

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

All under `/api/matches`. **Auth: required.** v1 is the **async duel, HTTP only** (`mode: "async"`; live WS is §9, not-v1). The full lifecycle, state machine, canvas size (1080×1080), visibility rules, and ratings live in **`GAME.md`**; the judge result shape lives in **`JUDGE.md`**. API.md carries only the HTTP edges.

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

Errors: `400 validation_failed` (bad `mode`), `401 unauthorized`, `429 rate_limited`.

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
> After the final submit `status` is `judging`; before it (opponent still drawing) `status` stays `drawing`. Same `drawingDeadline`/`serverTime` pair as `GET` (above), so the client re-anchors its countdown off this ack without a follow-up `GET`. The client then polls `GET /api/matches/{id}/result` (§8.4) for the verdict once `ready: true` (WS push replaces polling in §9, not-v1).

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
    "resolution": "judged",       // or "forfeit" — see below
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
- `winnerUserId` is the **resolved player id** (`matches.winner_player_id`) — the `game` module mapped the judge's positional `A`/`B`/`tie` onto it at submit time (`JUDGE.md` / `ARCHITECTURE.md` §5). `null` ⇔ `isTie: true`.
- `score` / `reason` come from the judge (`match_players.score`, `matches.judge_reason`). `judgedImageUrl` **stays `null`**: it *would* point at the server-rendered authoritative raster in object storage, but object storage is **deferred** (not built). The reveal shows the opponent's canvas via `GET …/players/{userId}/drawing` (below) + a client render instead, so no raster URL is needed for it; the field is kept for a future feed-thumbnail / render-offload use.
- `resolution` is `"judged"` (the normal path — both players submitted, the judge ran) or `"forfeit"` (the round deadline passed with exactly one submitter — that player won by default, full Elo, **no judge ran**, `GAME.md` §4.1/§8). On a forfeit, both players' `score` is `null` (no judge similarity was produced — never `0`); the forfeiting player's `drawingId` is `null` if they never submitted. The client branches its result copy on `resolution`, never on the free-text `reason`. Every completed match has a non-null `resolution` (historical pre-migration rows default to `"judged"`).
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

## 9. Live match-room WS — **NOT v1 (forward-looking sketch)**

> **Explicitly not implemented in v1.** The async duel (§8) needs only HTTP. This sketches the live-mode protocol so §8 doesn't paint us into a corner. Async and live **share one match lifecycle** (`ARCHITECTURE.md` §8) — live is a *delivery upgrade* (push instead of poll), not a second backend. Built only when a §9-of-ARCHITECTURE trigger fires.

- **Transport.** `coder/websocket`, an **in-process hub of match rooms** inside `server/` (`internal/ws`) — same binary, same auth, same Postgres. **Postgres stays the source of truth**; the hub pushes transitions, it does not own them.
- **Endpoint (sketch).** `GET /api/matches/{id}/ws` — upgrades to WS. **Auth: the same `jp_session` cookie** (the upgrade request carries it); the caller must be a player in the match or the upgrade is refused (`403`/close). `mode` becomes `"live"` for these matches.
- **Server → client events** (JSON frames, `{ "type": …, … }`), forward-looking:
  - `match_state` — full snapshot on join (mirrors `GET /api/matches/{id}`, with the same opponent-redaction rule until `done`).
  - `opponent_joined` — the second player connected.
  - `opponent_submitted` — opponent finished drawing (no canvas leaked — just the flag).
  - `judging` — both submitted; server is rendering + scoring.
  - `result` — verdict payload (mirrors `GET /api/matches/{id}/result`; both canvases revealed).
  - `opponent_left` / `abandoned` — disconnect / terminal abandon.
- **Client → server events** (sketch): `ping` (keepalive). The **actual drawing submit still goes over HTTP `POST /…/submit`** even in live mode — the document-validation + authoritative-render + trust boundary path is identical; WS only carries presence + state push. (Live *stroke streaming* between players, if ever wanted, is a separate, later seam — not part of this sketch.)
- **Reconnect.** A reconnecting client re-fetches `match_state`; no event replay buffer in the sketch (Postgres is authoritative — just re-snapshot).

These names/shapes are **indicative**, pinned only when live mode is actually built. v1 ships §1–§8.
