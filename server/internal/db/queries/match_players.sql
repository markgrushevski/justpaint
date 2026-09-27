-- name: GetMatchPlayer :one
-- One roster slot — used at submit to decide 403 (not a player) vs 409 (already
-- submitted). submitted_at != null ⇒ this player has already submitted.
select * from match_players
where match_id = $1 and user_id = $2;

-- name: StampSubmission :execrows
-- Record a player's submission. The `submitted_at is null` guard makes it
-- idempotent: a double-submit stamps zero rows (the handler answers 409).
update match_players
set drawing_id = $3, submitted_at = now()
where match_id = $1 and user_id = $2 and submitted_at is null;

-- name: CountUnsubmitted :one
-- How many roster slots have not submitted yet. Read under the match row lock
-- (GetMatchForUpdate) so the last-submit → judging flip can't race.
select count(*) from match_players
where match_id = $1 and submitted_at is null;

-- name: GetSubmissionsForJudging :many
-- The two submissions with rating and drawing document, ordered by
-- (submitted_at, user_id) for the stable A/B mapping (docs/GAME.md §7.1): row 0
-- = image A, row 1 = image B. No `nulls last` (unlike ListMatchPlayers): the
-- inner join admits only stamped rows, so submitted_at is never null here. A
-- left join would make the order nondeterministic.
select mp.user_id,
       mp.drawing_id,
       mp.submitted_at,
       u.rating,
       d.document
from match_players mp
join users u on u.id = mp.user_id
join drawings d on d.id = mp.drawing_id
where mp.match_id = $1
order by mp.submitted_at asc, mp.user_id asc;

-- name: SetPlayerScore :exec
-- Record the judge's per-player score and the Elo snapshot around the match
-- (docs/GAME.md §8).
update match_players
set score = $3, rating_before = $4, rating_after = $5
where match_id = $1 and user_id = $2;

-- name: GetMatchPlayerDrawing :one
-- The document a match participant (`target_user_id`) submitted, revealed to a
-- fellow participant (`viewer_user_id`) once the match is `done` — the
-- membership-gated reveal endpoint (docs/DECISIONS.md 2026-07-11). One row folds
-- three trust gates (viewer is a player, match is done, target is a submitted
-- player); any miss yields no row, so the service can't leak which gate failed.
select d.document
from drawings d
         join match_players tp
              on tp.drawing_id = d.id
                  and tp.match_id = sqlc.arg('match_id')
                  and tp.user_id = sqlc.arg('target_user_id')
                  -- Defence-in-depth backstops (belt-and-suspenders, not load-bearing today):
                  -- the drawing must be the target's OWN and linked to THIS match. The write
                  -- path already guarantees this (StampSubmission only ever points drawing_id at
                  -- a fresh CreateDrawing with owner_id = that player, match_id = this match), so
                  -- these predicates change nothing now — but they make the correctness hold
                  -- BY CONSTRUCTION, not by invariant, if a future write path ever diverges, and
                  -- they fail closed if the positional $2/$3 (target/viewer) binds ever swap.
                  and d.owner_id = sqlc.arg('target_user_id')
                  and d.match_id = sqlc.arg('match_id')
         join matches m
              on m.id = sqlc.arg('match_id')
                  and m.status = 'done'
where exists (select 1
              from match_players vp
              where vp.match_id = sqlc.arg('match_id')
                and vp.user_id = sqlc.arg('viewer_user_id'));

-- name: GetMatchPlayersForResolve :many
-- Per-player state the deadline resolver needs in one read: who submitted (and
-- their drawing), plus the live rating for forfeit Elo. Stable (submitted_at,
-- user_id) order — the same A/B seat ordering judging uses (docs/GAME.md §7.1).
select mp.user_id,
       mp.submitted_at,
       mp.drawing_id,
       u.rating
from match_players mp
join users u on u.id = mp.user_id
where mp.match_id = $1
order by mp.submitted_at asc nulls last, mp.user_id asc;
