-- name: CreatePracticeRun :one
-- Records the ATTEMPT, before the critic is called. The score and feedback stay
-- null until a verdict comes back; a row that keeps them is a judge call that was
-- spent and produced nothing, which still counts against the daily budget
-- (migration 00006, docs/GAME.md §4.3). Writing it afterwards instead would make
-- every failure free — and a failing critic is exactly when the quota drains.
insert into practice_runs (user_id, prompt_id)
values (sqlc.arg('user_id'), sqlc.arg('prompt_id'))
returning *;

-- name: SetPracticeRunVerdict :one
-- Stamps the critique onto an attempt this user owns. Scoped by user_id as well as
-- id, like every other write here: a row belonging to somebody else must not be
-- reachable, and `returning` lets the caller notice it wasn't (no row back) rather
-- than assume the update landed.
--
-- The casts pin both parameters non-null on the Go side. The columns are nullable
-- because an attempt starts without a verdict, but this statement only ever runs
-- WITH one, so a *float64 here would be a pointer that is never nil.
update practice_runs
set score = sqlc.arg('score')::double precision,
    feedback = sqlc.arg('feedback')::text
where id = sqlc.arg('id')
  and user_id = sqlc.arg('user_id')
returning *;
