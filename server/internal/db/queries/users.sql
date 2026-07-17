-- name: CreateUser :one
insert into users (login, password_hash, display_name)
values ($1, $2, $3)
returning *;

-- name: GetUserByLogin :one
select * from users
where login = $1;

-- name: GetUserByID :one
select * from users
where id = $1;

-- name: ListTopRatings :many
-- The leaderboard: the top-rated players with their win/loss record (docs/GAME.md §8,
-- docs/API.md §11). `login` is deliberately NOT selected — it may be an email, and the
-- leaderboard is world-readable to every authed user (privacy, same rule as
-- ListMatchPlayers). The INNER JOINs to match_players/matches mean only users with >=1
-- 'done' match appear, so players who have never finished a game fall out naturally —
-- no HAVING, no 0-games filter. status='done' counts forfeits (full-K Elo) and excludes
-- 'abandoned' (no Elo, no result). wins/losses derive from winner_player_id (null =
-- tie); ties are games_played-wins-losses, not a stored column. Tie-break on id asc
-- because every account starts at rating 1200, so a fresh ladder would otherwise order
-- nondeterministically.
select u.id,
       u.display_name,
       u.rating,
       count(*)::int as games_played,
       count(*) filter (where m.winner_player_id = u.id)::int as wins,
       count(*) filter (where m.winner_player_id is not null and m.winner_player_id <> u.id)::int as losses
from users u
join match_players mp on mp.user_id = u.id
join matches m on m.id = mp.match_id and m.status = 'done'
group by u.id, u.display_name, u.rating
order by u.rating desc, u.id asc
limit sqlc.arg('lim')::int;

-- name: ApplyRatingDelta :one
-- Atomically move a player's ladder rating by the match's Elo delta and return the
-- TRUE post-update rating. rating = rating + delta (never an absolute SET) so two
-- matches sharing a player resolving concurrently both land — the match-row lock
-- serializes per MATCH, not per USER (docs/NOTES.md).
update users set rating = rating + sqlc.arg('delta')::int, updated_at = now() where id = sqlc.arg('id') returning rating;
