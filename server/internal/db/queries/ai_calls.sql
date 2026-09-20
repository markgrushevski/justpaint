-- The AI-call ledger (migration 00007) — the whole daily budget, in six
-- statements that do not grow when a new AI feature ships. Compare
-- queries/judge_budget.sql, which this replaces: that one needed a new UNION arm
-- and a fresh "which column means spent" argument per feature.
--
-- A row's two shapes are two different FACTS that happen at two different
-- moments, which is why there are three write statements and not one:
--
--   * a player row says "this player was granted a round of this kind" — written
--     when the round starts;
--   * a provider row says "one request to the provider is about to be made" —
--     written where that is true, which for a duel is the flip to `judging` and
--     not the flip to `drawing`.
--
-- Only the single-actor features (practice, guess, assist) do both in the same
-- instant, and they get one statement that does both or neither.

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
-- Scoped by provider, not global-global: Google's exhaustion must not refuse a
-- feature served by another provider that still has quota (migration 00007).
select count(*)::bigint as calls
from ai_calls
where provider = sqlc.arg('provider')::text
  and created_at > now() - make_interval(secs => sqlc.arg('window_secs')::int);

-- name: RecordAICallUnderCap :execrows
-- The single-actor spend: one player row and one provider row, written together
-- or not at all, and only while that player is under their per-kind cap.
--
-- It is ONE statement for two reasons. The cheap one is the round trip. The
-- load-bearing one is atomicity without a transaction: the two rows are the two
-- halves of one call, and a failure between two separate inserts would leave a
-- provider row billing a request that the refusal means nobody will make.
--
-- The WHERE does not reference the UNION's rows, so it holds for both or for
-- neither: this inserts exactly 2 rows or exactly 0. Zero is the refusal, and
-- the caller turns it into the same *KindSpentError the advisory check returns.
--
-- What the condition buys, honestly: under READ COMMITTED two concurrent
-- statements can each still see the same pre-insert count and each insert, so
-- the cap is not exact. What shrinks is the window in which that can happen —
-- from "an advisory read, then a render, then a provider call" down to "one
-- statement" — which is the difference between a measured 12x overshoot under a
-- 25-request burst and an overshoot bounded by how many inserts genuinely
-- overlap inside Postgres. Exactness would need SERIALIZABLE or a per-user lock;
-- neither is worth serializing every AI call for a ceiling that already sits
-- below the provider's own.
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
-- The player half on its own: N player rows, no provider row, in one statement.
--
-- This is the duel's shape, and the only caller that needs it. Two players are
-- granted one round by a single act (the second seat filling), so their rows are
-- written together, inside the same transaction that starts the round — one
-- statement rather than one per player so a partial write is not a state the
-- ledger can reach.
--
-- Unconditional, unlike RecordAICallUnderCap, because there is nobody here to
-- refuse: the joiner was already checked before the transaction opened, and the
-- OTHER seat is being granted a round by somebody else's action. Dropping their
-- row would make the ledger undercount a round that really happened, and failing
-- the statement would strand a match on a cap that is not the caller's.
insert into ai_calls (user_id, kind)
select unnest(sqlc.arg('user_ids')::uuid[]), sqlc.arg('kind')::text;

-- name: RecordProviderAICall :exec
-- The provider half on its own: one row, one request about to be made.
--
-- Unconditional on purpose, and NOT gated on the global ceiling. By the time a
-- duel reaches judging the round has been played: two people drew for ninety
-- seconds, and refusing here would strand the match with nothing to show for it.
-- The global ceiling therefore stays advisory and is enforced at the check, back
-- when refusing still cost the players nothing.
insert into ai_calls (kind, provider)
values (sqlc.arg('kind')::text, sqlc.arg('provider')::text);

-- name: DeleteAICallsBefore :execrows
-- Retention sweep. Nothing is ever READ past the 24h window, but rows are kept a
-- week so "why did the ceiling refuse me last Tuesday" has an answer; a week at
-- the hard ceiling is a few thousand rows, which is not a storage problem.
--
-- It is a sequential scan: both indexes on this table are partial and lead on
-- user_id/provider, so neither can serve a bare created_at predicate. At a few
-- thousand rows that is cheaper than the third index it would take to avoid, and
-- it runs hourly on a background goroutine where nobody is waiting for it.
delete from ai_calls
where created_at < sqlc.arg('cutoff');
