-- +goose Up

-- A third terminal `resolution`: 'aborted' — the judging pass never returned after
-- judge_attempts hit the cap (a wedged judge, a missing/OOM-killed render worker),
-- so the round is closed as `done` with NO winner and NO rating change instead of
-- wedging in `judging` forever (docs/GAME.md §3/§4.1, docs/DECISIONS.md 2026-09-18).
-- Postgres cannot widen a check constraint in place, so 00004's constraint is
-- dropped and re-added. Named explicitly (not `if exists`) so a name mismatch fails
-- loudly at migrate time instead of leaving the narrow constraint silently in force.
alter table matches
    drop constraint matches_resolution_check,
    add constraint matches_resolution_check
        check (resolution in ('judged', 'forfeit', 'aborted'));

-- Backfill the staleness clock 00004 left null. A row already wedged in `judging`
-- when 00004 ran has judging_started_at = null, and `null <= now() - interval` is
-- null — so it matches NEITHER watchdog query (the re-fire sweep or the new abort
-- sweep) and is immortal. Stamping it now hands it to the watchdog: attempts are
-- still 0, so it re-fires normally and only then ages toward the cap.
update matches set judging_started_at = now()
    where status = 'judging' and judging_started_at is null;

-- +goose Down

-- Rows that already resolved as 'aborted' would violate the narrowed constraint, so
-- fold them back to the closest pre-00005 terminal shape: `abandoned`, the other
-- no-winner/no-Elo terminal. They lose the 'aborted' label, not the fact of ending.
update matches set status = 'abandoned', resolution = null, updated_at = now()
    where resolution = 'aborted';
alter table matches
    drop constraint matches_resolution_check,
    add constraint matches_resolution_check
        check (resolution in ('judged', 'forfeit'));
