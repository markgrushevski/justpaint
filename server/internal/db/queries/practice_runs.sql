-- name: CreatePracticeRun :one
-- Records the attempt before the critic is called. Score and feedback stay null
-- until a verdict comes back; a row that keeps them is a judge call that was
-- spent and produced nothing, which still counts against the daily budget
-- (docs/GAME.md §4.3). Writing it afterwards instead would make every failure
-- free, and a failing critic is exactly when the quota drains.
insert into practice_runs (user_id, prompt_id)
values (sqlc.arg('user_id'), sqlc.arg('prompt_id'))
returning *;

-- name: SetPracticeRunVerdict :one
-- Stamps the critique onto an attempt this user owns. Scoped by user_id as well
-- as id, like every write here, so `returning` with no row back tells the caller
-- the row wasn't reachable rather than that the update silently landed. The
-- casts pin both parameters non-null on the Go side: the columns are nullable
-- because an attempt starts without a verdict, but this statement only ever
-- runs with one.
update practice_runs
set score = sqlc.arg('score')::double precision,
    feedback = sqlc.arg('feedback')::text
where id = sqlc.arg('id')
  and user_id = sqlc.arg('user_id')
returning *;
