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
