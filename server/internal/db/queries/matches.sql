-- name: CreateMatch :one
-- A fresh open async match with one prompt pinned; mode/status use their column
-- defaults ('async'/'open'). docs/GAME.md §4.1.
insert into matches (prompt_id)
values ($1)
returning *;

-- name: GetMatch :one
select * from matches
where id = $1;

-- name: GetMatchForUpdate :one
-- Same as GetMatch but takes a row lock, serializing concurrent submits to one
-- match so the last-submit → judging flip is computed on a stable roster (two
-- players submitting at the same instant can't both miss "I'm last").
--
-- Also returns the DB clock (`server_now` = the tx-start now()) so the caller
-- compares the round deadline against ONE clock authority — the database's — on
-- the path that rejects a late submit, not the Go host wall clock (§2.4).
select *, now()::timestamptz as server_now from matches
where id = $1
for update;

-- name: SetMatchResult :one
-- Terminal write for the → done transition: winner (null = tie), the judge's
-- reason verbatim, and how the match resolved ('judged' or 'forfeit'), status
-- done (docs/GAME.md §4.1, §7.1, docs/DESIGN-PHASE3-LIVE.md §2.7).
update matches
set status = 'done', winner_player_id = $2, judge_reason = $3, resolution = $4, updated_at = now()
where id = $1
returning *;

-- name: UpdateMatchStatus :one
update matches
set status = $2, updated_at = now()
where id = $1
returning *;

-- name: FindOpenMatchToJoin :one
-- The oldest open async match the caller is NOT already in — the auto-join
-- candidate (docs/GAME.md §4.1, docs/DECISIONS.md "Matchmaking"). FOR UPDATE
-- SKIP LOCKED lets concurrent joiners each grab a different match instead of
-- colliding on one (the loser skips the locked row and creates its own).
select * from matches
where status = 'open'
  and mode = 'async'
  and not exists (
    select 1 from match_players mp
    where mp.match_id = matches.id and mp.user_id = $1
  )
order by created_at asc
limit 1
for update skip locked;

-- name: FindMyOpenMatch :one
-- The caller's own still-open match, if any — returned instead of stacking a
-- second open match when a waiting player taps "play" again.
select m.* from matches m
join match_players mp on mp.match_id = m.id
where m.status = 'open'
  and m.mode = 'async'
  and mp.user_id = $1
order by m.created_at asc
limit 1;

-- name: AddMatchPlayer :exec
-- Insert one roster slot. The composite PK (match_id, user_id) makes a double
-- join impossible.
insert into match_players (match_id, user_id)
values ($1, $2);

-- name: ListMatchPlayers :many
-- The roster for a match, with each player's optional display name. `login` is
-- deliberately NOT selected — it may be an email, and the opponent must not see
-- it (privacy). Ordered by submit time then user_id: the same stable ordering
-- the A/B→player mapping will use at judging (docs/GAME.md §7.1).
select mp.match_id,
       mp.user_id,
       mp.drawing_id,
       mp.score,
       mp.rating_before,
       mp.rating_after,
       mp.submitted_at,
       u.display_name
from match_players mp
join users u on u.id = mp.user_id
where mp.match_id = $1
order by mp.submitted_at asc nulls last, mp.user_id asc;

-- name: SetMatchDrawing :one
-- Start the round: the roster just filled, so flip open→drawing AND stamp the
-- server-authoritative deadline as now() + the round length (seconds). One clock
-- authority — the deadline every reader and the sweeper compare against is the
-- DB's own now() (docs/DESIGN-PHASE3-LIVE.md §2).
update matches
set status = 'drawing',
    drawing_deadline = now() + make_interval(secs => sqlc.arg('round_seconds')::int),
    updated_at = now()
where id = sqlc.arg('id')
returning *;

-- name: SetMatchJudging :one
-- Enter (or, for the stuck-judging watchdog, RE-enter) judging: stamp the start of
-- THIS attempt so staleness is measured per-attempt, and bump the retry counter.
update matches
set status = 'judging',
    judging_started_at = now(),
    judge_attempts = judge_attempts + 1,
    updated_at = now()
where id = $1
returning *;

-- name: SetMatchAbandoned :one
-- Terminal, no result: nobody submitted before the deadline (or an open match was
-- reaped). No scores, no rating change (docs/GAME.md §4.1).
update matches
set status = 'abandoned', updated_at = now()
where id = $1
returning *;

-- name: ListExpiredDrawingMatches :many
-- The sweeper's work list: live rounds whose deadline has passed, oldest first.
-- FOR UPDATE SKIP LOCKED lets a sweep racing a real submit (or a second sweeper)
-- partition the set instead of colliding; each id is then resolved in its own tx.
select id from matches
where status = 'drawing' and drawing_deadline <= now()
order by drawing_deadline
limit $1
for update skip locked;

-- name: ListStuckJudgingMatches :many
-- Judging rows wedged past the stale window with retries left — a crashed/hung
-- judge attempt to re-fire (docs/DESIGN-PHASE3-LIVE.md §2.6). Staleness is measured
-- against judging_started_at (the current attempt), not updated_at.
select id from matches
where status = 'judging'
  and judging_started_at <= now() - make_interval(secs => sqlc.arg('stale_secs')::int)
  and judge_attempts < sqlc.arg('max_attempts')::int
order by judging_started_at
limit sqlc.arg('lim')::int
for update skip locked;

-- name: ListStaleOpenMatches :many
-- Open matches nobody joined within the TTL — reaped to abandoned so a ghost can't
-- later ambush a fresh joiner (docs/DESIGN-PHASE3-LIVE.md §2.6, §5 Q9).
select id from matches
where status = 'open' and created_at <= now() - make_interval(secs => sqlc.arg('ttl_secs')::int)
order by created_at
limit sqlc.arg('lim')::int
for update skip locked;
