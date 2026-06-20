# Ideas & backlog (non-blocking)

> Parking lot for improvements surfaced mid-build that are **deliberately deferred** — not bugs, not Phase-1 blockers. Each item has a one-line rationale + a rough *when*. Promote an item into `ROADMAP.md` when its phase goes active. Hard, already-made trade-offs live in `DECISIONS.md`; this is the softer "good ideas, later" list.

## Auth / identity
- **Login charset/format validation** — `login` is currently validated by **length only** (3–254 chars). Add format rules: if it contains `@`, validate as an email; otherwise restrict a nickname to `[a-zA-Z0-9_.-]` (no spaces, no emoji). This is the real gap (vs. the email/nickname *split*, which isn't needed). *When:* cheap — fold into the next auth touch.
- **Optional separate `email` column** — split the sign-in handle (`login`) from a verified contact `email`, unlocking password reset, email verification, notifications. *When:* only when an email flow is actually built (YAGNI until then). Until then the single `login` (email-or-nickname, citext, case-insensitive) stands as decided.

## Observability (server)
- **Request-id correlation** — generate (or read `X-Request-Id`), attach to the slog context and echo in the response header, so every log line of one request correlates. *When:* next backend hardening pass; cheap, high value.
- **Prometheus `/metrics`** — `prometheus/client_golang`: a metrics middleware (`http_requests_total{method,route,status}`, a latency histogram, in-flight gauge), `pgxpool.Stat()` gauges, Go runtime collectors; Grafana dashboards on top. *When:* when dashboards are wanted / for portfolio depth.
- **Loki + Grafana (logs)** — the JSON-to-stdout slog is already Loki-friendly; shipping (promtail/alloy → Loki → Grafana) is an **ops** concern, not code. *When:* deploy time.
- **OpenTelemetry tracing** — traces → Tempo/Jaeger, viewable in Grafana. *When:* later; overkill for v1.

## Drawings / API
- **Rate limiting (429)** — already tracked in `DECISIONS.md` (deferred). The real remaining abuse vector for drawings (spamming valid-but-heavy documents), **not** SQL injection (queries are fully parameterized via sqlc; ownership is enforced in every `WHERE owner_id`). *When:* before any public exposure / Phase 3.
- **Server-generated `thumbnail_url` only** — when thumbnails land, store **only** a server-side object-storage URL, never a client-supplied one (avoids stored-SSRF / XSS via a poisoned URL). *When:* when thumbnails are implemented (Phase 2/3).

## Frontend / build
- **Replace vendored oriui with the published npm package** — `apps/web` currently `file:`-links a local copy of oriui under `vendor/oriui/` (`@oriui/css` + `@oriui/vue` + `@oriui/headless`, alpha.1), committed temporarily because oriui isn't on the public registry yet (and was renamed `@oriui/ui` → `@oriui/vue`). When you publish: delete `vendor/oriui/`, swap the three `file:` deps for versioned ones, drop the `.gitignore` vendor exception, and re-check imports. *When:* after you `npm publish` oriui.
- **Point `apps/web/.env` at the Go server** — `VITE_URL_API` still targets the old NestJS port `:8888`; the Go server listens on `:8080`. *When:* when wiring the editor save/load round-trip.

## Orchestration / process
- **Cross-domain parallel agents** — backend (`server/`) and frontend (`packages/` + `apps/`) share no files and both validate against the **frozen contract** (`docs/DOCUMENT-FORMAT.md`), so they can run on **parallel branches**; integration (review + `--no-ff` merge + ROADMAP update) stays **serialized** through one orchestrator. Within a single domain (e.g. the editor's tools), parallelize with git-worktree isolation or sequential commits to avoid same-file conflicts. *When:* as work volume warrants; the editor tool-set is the first good fan-out candidate.
