-- name: CreateMatch :one
-- One prompt pinned per match; mode/status take their column defaults
-- ('async'/'open', docs/GAME.md §4.1).
insert into matches (prompt_id)
values ($1)
returning *;

-- name: GetMatch :one
select * from matches
where id = $1;

-- name: GetMatchForUpdate :one
-- Same as GetMatch but takes a row lock, so two simultaneous submits can't each
-- see the other as unsubmitted and both miss the last-submit → judging flip
-- (docs/NOTES.md). Also returns the DB clock (`server_now`, the tx-start now())
-- so a late-submit check compares against one clock authority — the database's,
-- not the Go host's wall clock.
select *, now()::timestamptz as server_now from matches
where id = $1
for update;

-- name: SetMatchResult :one
-- Terminal write for the → done transition: winner (null = tie), the judge's
-- reason verbatim, and how the match resolved ('judged' or 'forfeit'), status
-- done (docs/GAME.md §4.1, §7.1).
update matches
set status = 'done', winner_player_id = $2, judge_reason = $3, resolution = $4, updated_at = now()
where id = $1
returning *;

-- name: UpdateMatchStatus :one
update matches
set status = $2, updated_at = now()
where id = $1
returning *;

-- name: LockMatchmaking :exec
-- One transaction-scoped lock for every matchmaking decision, so two players pressing
-- play at once see each other's open match, and one player's parallel presses can't
-- seat them twice (docs/GAME.md §4.1).
select pg_advisory_xact_lock(7101);

-- name: FindOpenMatchToJoin :one
-- The oldest open async match the caller is not already in, within the open TTL and
-- with its creator seen within the pulse window (docs/GAME.md §4.1). Runs under
-- LockMatchmaking; the row lock keeps the reaper and a cancel off it until commit.
select * from matches
where status = 'open'
  and mode = 'async'
  and created_at > now() - make_interval(secs => sqlc.arg('ttl_secs')::int)
  and not exists (
    select 1 from match_players mp
    where mp.match_id = matches.id and mp.user_id = sqlc.arg('user_id')
  )
  and exists (
    select 1 from match_players mp
    where mp.match_id = matches.id
      and mp.seen_at > now() - make_interval(secs => sqlc.arg('pulse_secs')::int)
  )
order by created_at asc
limit 1
for update skip locked;

-- name: FindMyLiveMatch :one
-- The caller's own match that is still in play — open (within the TTL), drawing or
-- judging — so pressing play again, a reload or a second tab returns it instead of
-- starting another.
select m.* from matches m
join match_players mp on mp.match_id = m.id
where mp.user_id = sqlc.arg('user_id')
  and (m.status in ('drawing', 'judging')
       or (m.status = 'open' and m.created_at > now() - make_interval(secs => sqlc.arg('ttl_secs')::int)))
order by m.created_at desc
limit 1;

-- name: TouchMatchPlayer :exec
-- The pulse: a player's poll of their open match.
update match_players set seen_at = now()
where match_id = $1 and user_id = $2;

-- name: AddMatchPlayer :exec
-- The composite PK (match_id, user_id) makes a double join impossible.
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
-- Flips open→drawing and stamps the deadline as now() + the round length, off
-- the DB's own clock — the one authority every reader and the sweeper compare
-- against (docs/GAME.md §4.1).
update matches
set status = 'drawing',
    drawing_deadline = now() + make_interval(secs => sqlc.arg('round_seconds')::int),
    updated_at = now()
where id = sqlc.arg('id')
returning *;

-- name: SetMatchJudgingQueued :one
-- Enter judging with no pass running yet: both drawings are in but no judging slot
-- was free. Nothing is stamped or counted; the sweeper starts the pass when a slot
-- frees (ListStuckJudgingMatches).
update matches
set status = 'judging',
    judging_started_at = null,
    updated_at = now()
where id = $1
returning *;

-- name: SetMatchJudging :one
-- Start a judging pass (the first, or a re-fire): stamp the start of this attempt so
-- staleness is measured per-attempt, and bump the retry counter.
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
-- The sweeper's work list: live rounds whose deadline has passed, oldest first. The
-- list runs outside a transaction, so SKIP LOCKED only passes over rows a submit holds
-- right now; each id is re-checked under its own row lock (docs/NOTES.md).
select id from matches
where status = 'drawing' and drawing_deadline <= now()
order by drawing_deadline
limit $1
for update skip locked;

-- name: ListStuckJudgingMatches :many
-- Judging rows to start: queued ones (no pass ran yet, judging_started_at null) and
-- ones wedged past the stale window with retries left (docs/GAME.md §4.1). Staleness
-- is measured against the current attempt, not updated_at. Exact complement:
-- ListExhaustedJudgingMatches, which a null start never matches.
select id from matches
where status = 'judging'
  and (judging_started_at is null
       or judging_started_at <= now() - make_interval(secs => sqlc.arg('stale_secs')::int))
  and judge_attempts < sqlc.arg('max_attempts')::int
order by judging_started_at nulls first
limit sqlc.arg('lim')::int
for update skip locked;

-- name: ListExhaustedJudgingMatches :many
-- The complement of ListStuckJudgingMatches: retries used up, last attempt
-- stale. Swept to `done` + resolution 'aborted', no winner, no Elo (docs/GAME.md
-- §4.1, docs/DECISIONS.md 2026-09-18). Same partial index and stale window, so a
-- row is in exactly one list.
select id from matches
where status = 'judging'
  and judging_started_at <= now() - make_interval(secs => sqlc.arg('stale_secs')::int)
  and judge_attempts >= sqlc.arg('max_attempts')::int
order by judging_started_at
limit sqlc.arg('lim')::int
for update skip locked;

-- name: ListStaleOpenMatches :many
-- Open matches past the TTL or whose creator stopped polling — reaped to abandoned
-- (docs/GAME.md §4.1).
select id from matches
where status = 'open'
  and (created_at <= now() - make_interval(secs => sqlc.arg('ttl_secs')::int)
       or not exists (
         select 1 from match_players mp
         where mp.match_id = matches.id
           and mp.seen_at > now() - make_interval(secs => sqlc.arg('pulse_secs')::int)
       ))
order by created_at
limit sqlc.arg('lim')::int
for update skip locked;
