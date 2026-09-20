-- The AI-call ledger (migration 00007) — the whole daily budget, in four
-- statements that do not grow when a new AI feature ships. Compare
-- queries/judge_budget.sql, which this replaces: that one needed a new UNION arm
-- and a fresh "which column means spent" argument per feature.

-- name: RecordAICall :exec
-- Append one ledger row. Called by aibudget.Spend, never directly: exactly one
-- row per call carries the provider and one row per billed player carries the
-- user, and the pairing is that function's job (migration 00007).
insert into ai_calls (user_id, kind, provider)
values (sqlc.narg('user_id'), sqlc.arg('kind'), sqlc.narg('provider'));

-- name: CountUserKindCallsInWindow :one
-- One player's spend on ONE kind inside the rolling window — the per-kind,
-- per-user ceiling (docs/GAME.md §4.3).
--
-- Rows with a null user_id are the provider-side halves and are excluded by the
-- equality itself (null = $1 is null, never true), which is also why the
-- supporting index is partial.
select count(*)::bigint as calls
from ai_calls
where user_id = sqlc.arg('user_id')::uuid
  and kind = sqlc.arg('kind')::text
  and created_at > now() - make_interval(secs => sqlc.arg('window_secs')::int);

-- name: CountProviderCallsInWindow :one
-- How much of ONE provider's daily quota this service has spent inside the
-- rolling window, across every kind. The global half of the ceiling.
--
-- Scoped by provider, not global-global: Google's exhaustion must not refuse an
-- Anthropic-backed feature that still has quota (migration 00007).
select count(*)::bigint as calls
from ai_calls
where provider = sqlc.arg('provider')::text
  and created_at > now() - make_interval(secs => sqlc.arg('window_secs')::int);

-- name: DeleteAICallsBefore :execrows
-- Retention sweep. Nothing is ever READ past the 24h window, but rows are kept a
-- week so "why did the ceiling refuse me last Tuesday" has an answer; a week at
-- the hard ceiling is a few thousand rows, which is not a storage problem.
delete from ai_calls
where created_at < sqlc.arg('cutoff');
