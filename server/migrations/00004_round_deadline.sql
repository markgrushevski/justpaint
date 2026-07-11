-- +goose Up

-- Server-authoritative round deadline + forfeit/abandon resolution (docs/GAME.md
-- §3/§4, docs/DESIGN-PHASE3-LIVE.md §2). Until now the 90s round timer lived only
-- on the client and a match whose opponent never submitted had NO exit from
-- `drawing` — this makes the deadline a real column the server stamps, enforces,
-- and sweeps.
alter table matches
    add column drawing_deadline   timestamptz,                                    -- null while `open`; stamped now()+round at open→drawing
    add column resolution         text check (resolution in ('judged', 'forfeit')), -- how a `done` match ended; null for open/drawing/abandoned
    add column judge_attempts     int not null default 0,                          -- stuck-judging watchdog retry counter
    add column judging_started_at timestamptz;                                     -- start of the current judge attempt (staleness clock)

-- Backfill BEFORE indexing so no existing row is stranded or mistyped:
--   * a live `drawing` row with a null deadline would never be swept (immortal) —
--     give it one so it can still resolve.
--   * a historical `done` row must not present resolution=NULL to a non-null DTO field.
update matches set drawing_deadline = now() + interval '90 seconds'
    where status = 'drawing' and drawing_deadline is null;
update matches set resolution = 'judged'
    where status = 'done' and resolution is null;

-- Sweeper scans only live, already-expired rows.
create index matches_deadline_sweep_idx
    on matches (drawing_deadline) where status = 'drawing';

-- Stuck-judging watchdog scans only rows wedged in `judging`.
create index matches_judging_stuck_idx
    on matches (judging_started_at) where status = 'judging';

-- Open-match reaper scans only never-joined rows.
create index matches_open_stale_idx
    on matches (created_at) where status = 'open';

-- +goose Down

drop index if exists matches_open_stale_idx;
drop index if exists matches_judging_stuck_idx;
drop index if exists matches_deadline_sweep_idx;
alter table matches
    drop column judging_started_at,
    drop column judge_attempts,
    drop column resolution,
    drop column drawing_deadline;
