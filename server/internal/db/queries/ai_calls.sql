-- The AI-call ledger (docs/GAME.md §4.3): one table for every AI feature's daily
-- budget. A row bills either a player (their per-kind allowance) or a provider
-- (its quota), never both, since for a duel the two happen at different moments.

-- name: CountUserKindCallsInWindow :one
-- One player's spend on one kind inside the rolling window — the per-kind cap
-- (docs/GAME.md §4.3). Rows with a null user_id are provider rows, excluded by
-- the equality itself (null = $1 is never true) — why the index is partial.
select count(*)::bigint as calls
from ai_calls
where user_id = sqlc.arg('user_id')::uuid
  and kind = sqlc.arg('kind')::text
  and created_at > now() - make_interval(secs => sqlc.arg('window_secs')::int);

-- name: CountProviderCallsInWindow :one
-- One provider's spend inside the rolling window, across every kind — the
-- global half of the ceiling (docs/GAME.md §4.3), scoped per provider so one
-- exhausted provider can't refuse a feature served by another.
select count(*)::bigint as calls
from ai_calls
where provider = sqlc.arg('provider')::text
  and created_at > now() - make_interval(secs => sqlc.arg('window_secs')::int);

-- name: RecordAICallUnderCap :execrows
-- Writes the player and provider rows together, only while under the per-kind
-- cap — one statement, so a crash between two inserts can't bill a provider for
-- a request nobody makes. Inserts 2 rows or 0; the caller turns 0 into the same
-- *KindSpentError the advisory check returns. Not exact under concurrent
-- writers (docs/GAME.md §4.3, "How exact the two halves are").
insert into ai_calls (user_id, kind, provider)
select r.user_id, sqlc.arg('kind')::text, r.provider
from (
    -- the player-side row: whose allowance this call spends
    select sqlc.arg('user_id')::uuid as user_id, null::text as provider
    union all
    -- the provider-side row: the one request this call makes
    select null::uuid, sqlc.arg('provider')::text
) as r
where (
    select count(*)
    from ai_calls
    where user_id = sqlc.arg('user_id')::uuid
      and kind = sqlc.arg('kind')::text
      and created_at > now() - make_interval(secs => sqlc.arg('window_secs')::int)
) < sqlc.arg('cap')::int;

-- name: RecordPlayerAICalls :exec
-- The duel's player half: N player rows, no provider row, in one statement so
-- the roster is billed together when the second seat fills the round (docs/GAME.md
-- §4.3). Unconditional, unlike RecordAICallUnderCap: the joiner was already
-- checked before the transaction opened, and the other seat's round is granted
-- by that same action, not by them, so dropping their row would undercount a
-- round that really happened.
insert into ai_calls (user_id, kind)
select unnest(sqlc.arg('user_ids')::uuid[]), sqlc.arg('kind')::text;

-- name: RecordProviderAICall :exec
-- The duel's provider half: one row per judging pass. Unconditional and not
-- gated on the global ceiling — by the time judging is reached the round has
-- been played, and refusing here would strand the match for nothing (docs/GAME.md
-- §4.3).
insert into ai_calls (kind, provider)
values (sqlc.arg('kind')::text, sqlc.arg('provider')::text);

-- name: DeleteAICallsBefore :execrows
-- Retention sweep to a week (docs/GAME.md §4.3). Runs as a sequential scan: both
-- indexes on this table are partial and lead on user_id/provider, so neither can
-- serve a bare created_at predicate.
delete from ai_calls
where created_at < sqlc.arg('cutoff');
