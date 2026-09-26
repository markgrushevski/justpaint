-- +goose Up

-- One ledger for every AI feature's daily budget, replacing per-domain-table
-- counting (docs/DECISIONS.md 2026-09-20 "One AI-call ledger", docs/GAME.md
-- §4.3). A row bills a player (user_id set) or a provider (provider set), never
-- both — a duel's two halves happen at different moments, so a single combined
-- row would either double- or undercount.
create table ai_calls (
    id         uuid        primary key default gen_random_uuid(),
    -- Null: this is the provider-side row of a call.
    user_id    uuid references users (id),
    -- Which feature spent the call. No check constraint: it's a Go constant
    -- bound at the composition root, so the compiler catches a typo and a new
    -- AI feature needs no migration.
    kind       text        not null,
    -- Null: this is a player-side row. Non-null names whose quota was spent
    -- ('google:<model>', 'collaborator', …) — the global ceiling is per
    -- provider, so one running dry can't throttle another.
    provider   text,
    created_at timestamptz not null default now(),
    -- The invariant the Spend function upholds: a row billing neither a player
    -- nor a provider is a bug, not a record.
    constraint ai_calls_bills_something check (user_id is not null or provider is not null)
);

-- The two counts the budget asks and nothing else: both indexes are partial, so
-- neither carries the other's rows.
create index ai_calls_user_kind_created_idx on ai_calls (user_id, kind, created_at desc)
    where user_id is not null;
create index ai_calls_provider_created_idx on ai_calls (provider, created_at desc)
    where provider is not null;

-- No backfill: the window is 24h, so continuity would cost re-deriving the exact
-- "which column means spent" judgement this table exists to delete. Worst case
-- is one extra day's allowance (docs/DECISIONS.md 2026-09-20).

-- +goose Down
drop index if exists ai_calls_provider_created_idx;
drop index if exists ai_calls_user_kind_created_idx;
drop table if exists ai_calls;
