-- +goose Up

-- Practice: one player, one prompt, one scored drawing — no opponent.
--
-- Deliberately NOT a row in `matches` with mode='solo'. The duel lifecycle is
-- intricate *because* two people wait on each other: matchmaking, a shared
-- deadline, forfeit, abandonment, the stuck-judging watchdog. None of that has
-- meaning for one player, and `decideExpiry` says so itself — a single-seat
-- drawing round falls into its "malformed roster" branch and is flipped to
-- judging, where runJudging demands exactly two submissions and wedges. Reusing
-- `matches` would mean surgery on the most delicate state machine here to
-- support a mode that needs none of it.
--
-- So: a flat record, written by the request that scores it. No status column, no
-- sweeper, no deadline. It exists for two reasons — the daily judge budget must
-- count these calls (they spend the same quota as a duel), and a player's result
-- should survive the response that produced it.
create table practice_runs (
    id         uuid        primary key default gen_random_uuid(),
    user_id    uuid        not null references users (id),
    prompt_id  uuid        not null references prompts (id),
    -- Null until the critic answers. A row with a null score is an ATTEMPT that
    -- spent a judge call and failed; it still counts against the budget, which is
    -- the conservative direction (docs/GAME.md §4.3).
    score      double precision,
    feedback   text,
    created_at timestamptz not null default now()
);

-- The budget's per-player count and the global count both filter on a rolling
-- window; the global one also unions with `matches`. Both read created_at.
create index practice_runs_user_created_idx on practice_runs (user_id, created_at desc);
create index practice_runs_created_idx on practice_runs (created_at desc);

-- +goose Down
drop index if exists practice_runs_created_idx;
drop index if exists practice_runs_user_created_idx;
drop table if exists practice_runs;
