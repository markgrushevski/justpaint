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
-- Every spend writes ONE row per provider request plus ONE row per billed player,
-- with no special cases:
--
--   practice/guess/assist → 2 rows: (null, provider) + (user, null)
--   duel                  → 3 rows: (null, provider) + (userA, null) + (userB, null)
--
-- The split exists because a duel costs the provider ONE request but costs TWO
-- players a day's allowance each. Rolling both facts into per-player rows would
-- make the global count charge a duel twice, halving the real ceiling for the
-- product's main mode; rolling them into one row would undercount the player who
-- is not named on it. Two nullable columns and one check keep both counts exact
-- with no DISTINCT, no divisor and no join.
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
    -- names WHOSE free tier was spent ('google', 'anthropic', …), because the
    -- global ceiling is per provider — Google running dry must never throttle
    -- Anthropic, or the reverse.
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
