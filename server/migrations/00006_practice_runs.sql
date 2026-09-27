-- +goose Up

-- Practice: one player, one prompt, one scored drawing, no opponent — and
-- deliberately not a `matches` row (docs/DECISIONS.md 2026-09-20). The duel
-- lifecycle exists because two people wait on each other, and a single-seat
-- round would wedge `runJudging`, which demands two submissions. A flat record
-- instead: no status column, no sweeper, no deadline.
create table practice_runs (
    id         uuid        primary key default gen_random_uuid(),
    user_id    uuid        not null references users (id),
    prompt_id  uuid        not null references prompts (id),
    -- Null until the critic answers; an attempt that spent a judge call and
    -- failed still counts against the budget (docs/GAME.md §4.3).
    score      double precision,
    feedback   text,
    created_at timestamptz not null default now()
);

-- Both budget counts (per-player, global) filter on a rolling window and read
-- created_at (docs/GAME.md §4.3).
create index practice_runs_user_created_idx on practice_runs (user_id, created_at desc);
create index practice_runs_created_idx on practice_runs (created_at desc);

-- +goose Down
drop index if exists practice_runs_created_idx;
drop index if exists practice_runs_user_created_idx;
drop table if exists practice_runs;
