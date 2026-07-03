-- name: CreateMatch :one
-- A fresh open async match with one prompt pinned; mode/status use their column
-- defaults ('async'/'open'). docs/GAME.md §4.1.
insert into matches (prompt_id)
values ($1)
returning *;

-- name: GetMatch :one
select * from matches
where id = $1;

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
