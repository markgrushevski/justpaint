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
-- The two submissions with each player's live rating and their drawing document,
-- ordered by (submitted_at, user_id) — the stable A/B ordering (docs/GAME.md §7.1):
-- row 0 = image A, row 1 = image B. No `nulls last` (unlike ListMatchPlayers): the
-- INNER JOIN on drawings admits only stamped rows, so submitted_at is never null
-- here. Don't switch this to a LEFT JOIN or the A/B bind becomes nondeterministic.
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

-- name: UpdateUserRating :exec
update users
set rating = $2, updated_at = now()
where id = $1;
