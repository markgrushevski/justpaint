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
-- The leaderboard: top-rated players with their win/loss record (docs/GAME.md
-- §8, docs/API.md §11). `login` is deliberately not selected — same privacy rule
-- as ListMatchPlayers. `is distinct from 'aborted'` (not `!=`) so a null
-- resolution still counts.
select u.id,
       u.display_name,
       u.rating,
       count(*)::int as games_played,
       count(*) filter (where m.winner_player_id = u.id)::int as wins,
       count(*) filter (where m.winner_player_id is not null and m.winner_player_id <> u.id)::int as losses
from users u
join match_players mp on mp.user_id = u.id
join matches m on m.id = mp.match_id and m.status = 'done' and m.resolution is distinct from 'aborted'
group by u.id, u.display_name, u.rating
order by u.rating desc, u.id asc
limit sqlc.arg('lim')::int;

-- name: ApplyRatingDelta :one
-- Moves a player's rating by the match's Elo delta and returns the true
-- post-update value. `rating + delta`, never an absolute set, so two matches
-- sharing a player and resolving concurrently both land — the match row lock
-- serializes per match, not per user (docs/NOTES.md).
update users set rating = rating + sqlc.arg('delta')::int, updated_at = now() where id = sqlc.arg('id') returning rating;
