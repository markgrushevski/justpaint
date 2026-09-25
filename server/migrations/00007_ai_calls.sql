-- +goose Up

-- One ledger for every AI call the service makes, replacing the count-by-domain-
-- table budget in internal/game (migration 00006 era, queries/judge_budget.sql).
--
-- # Why the old way had to go
--
-- The budget used to be derived by unioning the tables a feature happened to
-- write: a duel counted through `matches`/`match_players`, a practice run through
-- `practice_runs`. Each union arm needed a judgement about WHICH lifecycle column
-- means "a call was actually spent" — `drawing_deadline is not null` for a duel's
-- per-player half, `judging_started_at is not null` for its global half — and
-- those judgements were subtle, correct, and re-derived per table.
--
-- Two of the AI features now on the roadmap simply cannot be counted that way:
--
--   * guess (/draw asks the AI what you drew) has NO row anywhere — the drawing
--     may never be saved. Inventing a table so it can be counted IS a ledger,
--     just a worse one with a single column of interest.
--   * assist has no row either. It is bounded only by an in-process token bucket
--     (internal/assist/ratelimit.go), which the host resets on every deploy and
--     every wake from idle — so its ceiling has never actually held.
--
-- # Shape
--
-- A row records ONE of two facts, and which one it is says so in its own columns:
--
--   (user, null)     this player was granted a call of this kind — spends THEIR day
--   (null, provider) one request to this provider is about to be made — spends OURS
--
-- The two are separate columns rather than one row because they are separate
-- facts that do not always happen together, and for a duel they do not even
-- happen at the same moment:
--
--   practice/guess/assist → 2 rows, written together in one statement
--   duel                  → 2 player rows when the roster fills and the round
--                           starts, then 1 provider row per judging pass
--
-- That timing is the whole point. A duel that ends in a forfeit or is abandoned
-- never reaches the judge, so it writes no provider row at all; a duel whose
-- judging gets stuck and is re-fired writes one per pass while still costing its
-- two players a single round. An earlier version of this file billed all of it at
-- round start and claimed "ONE row per provider request" — which over-billed every
-- forfeit and under-billed every retry, both measured. (Still uncounted, and
-- deliberately: the judge client's own retries inside ONE pass. Billing those would
-- give the frozen Judge contract a database dependency.)
--
-- Keeping the facts apart is also what keeps both counts exact with no DISTINCT,
-- no divisor and no join: one duel costs the provider one request but costs two
-- players a day's allowance each, and no single-row encoding states both.
create table ai_calls (
    id         uuid        primary key default gen_random_uuid(),
    -- Null means this row bills no player: it is the provider-side row of a call.
    user_id    uuid references users (id),
    -- Which feature spent the call ('duel', 'practice', …). Deliberately NOT a
    -- check constraint or an enum: the value is a Go constant bound once at the
    -- composition root and never reaches here from a request, so the compiler
    -- already rejects a typo. A check would buy nothing and would make every new
    -- AI feature need a migration — exactly the friction this table removes.
    kind       text        not null,
    -- Null means this row bills no provider: it is a player-side row. Non-null
    -- names WHOSE quota was spent ('google:<model>', 'collaborator', …), because
    -- the global ceiling is per provider — one running dry must never throttle
    -- another.
    provider   text,
    created_at timestamptz not null default now(),
    -- The invariant the Spend function upholds, written where it cannot drift: a
    -- row that bills neither a player nor a provider is a bug, not a record.
    constraint ai_calls_bills_something check (user_id is not null or provider is not null)
);

-- The two counts the budget asks, and nothing else. Both are partial: a player
-- row never answers the global question and a provider row never answers the
-- per-player one, so neither index should carry the other's rows.
create index ai_calls_user_kind_created_idx on ai_calls (user_id, kind, created_at desc)
    where user_id is not null;
create index ai_calls_provider_created_idx on ai_calls (provider, created_at desc)
    where provider is not null;

-- No backfill from `matches` / `practice_runs`. The window is 24h, so a backfill
-- would buy at most one day of continuity at the price of re-deriving the exact
-- "which column means spent" judgement this table exists to delete. Every counter
-- starts at zero on the deploy that lands this; worst case is one extra day's
-- allowance, bounded and one-time (greenfield — CLAUDE.md).

-- +goose Down
drop index if exists ai_calls_provider_created_idx;
drop index if exists ai_calls_user_kind_created_idx;
drop table if exists ai_calls;
