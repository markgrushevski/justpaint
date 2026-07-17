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

-- name: ApplyRatingDelta :one
-- Atomically move a player's ladder rating by the match's Elo delta and return the
-- TRUE post-update rating. rating = rating + delta (never an absolute SET) so two
-- matches sharing a player resolving concurrently both land — the match-row lock
-- serializes per MATCH, not per USER (docs/NOTES.md).
update users set rating = rating + sqlc.arg('delta')::int, updated_at = now() where id = sqlc.arg('id') returning rating;
