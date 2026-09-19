-- The two halves of the daily judge-call budget (docs/GAME.md §4.3), together in
-- one file because they are one feature and each now spans several tables: a judge
-- call is spent by a DUEL entering judging and by a PRACTICE RUN being critiqued,
-- and a ceiling that counted only duels would be no ceiling at all the moment
-- practice shipped. They used to live in matches.sql / match_players.sql, back when
-- a duel was the only thing that could spend the quota.

-- name: CountPlayerJudgeCallsInWindow :one
-- One player's share of the budget: how many judge calls they have caused inside
-- the rolling window, across both modes. The per-player daily cap compares against
-- this (docs/GAME.md §4.3).
--
-- Duels: the "actually started" test is `drawing_deadline is not null`, NOT
-- `status <> 'open'`. The stale-open reaper flips a match nobody ever joined to
-- `abandoned`, and that duel cost nothing — billing a player for having waited
-- alone would be a cap on patience rather than on judge calls. The deadline is
-- stamped at exactly one site (SetMatchDrawing, open→drawing), which makes it the
-- honest marker for "an opponent showed up and the round ran".
--
-- Practice: every row counts, including one whose score is still null. A null score
-- is an ATTEMPT that spent a judge call and failed, and counting it is the
-- conservative direction — the row is written BEFORE the critic is called precisely
-- so a failure cannot become free (migration 00006).
select ((
    select count(*)
    from match_players mp
    join matches m on m.id = mp.match_id
    where mp.user_id = sqlc.arg('user_id')
      and m.drawing_deadline is not null
      and m.created_at > now() - make_interval(secs => sqlc.arg('window_secs')::int)
  ) + (
    select count(*)
    from practice_runs pr
    where pr.user_id = sqlc.arg('user_id')
      and pr.created_at > now() - make_interval(secs => sqlc.arg('window_secs')::int)
  ))::bigint as judge_calls;

-- name: CountJudgeCallsInWindow :one
-- The global half: how much of the external judge's quota this service has spent
-- inside the rolling window (or is about to — a match still in `judging` has its
-- call in flight).
--
-- Duels are counted by judging_started_at, which is stamped by exactly one
-- statement (SetMatchJudging) and is therefore the only column that means "the
-- judge was called". Counting `done` rows instead would bill us for forfeits, which
-- resolve drawing→done without the judge ever seeing them. A match that never
-- reached judging has it null, and `null > x` is null, so it simply never matches —
-- no is-not-null guard needed. Anchoring on that column rather than created_at also
-- means a stuck-judging re-fire ages from its latest attempt, not its first.
--
-- Practice has no such transition to anchor on: the call is made inside the request
-- that writes the row, so created_at IS the instant the quota was spent.
select ((
    select count(*)
    from matches
    where judging_started_at > now() - make_interval(secs => sqlc.arg('window_secs')::int)
  ) + (
    select count(*)
    from practice_runs
    where created_at > now() - make_interval(secs => sqlc.arg('window_secs')::int)
  ))::bigint as judge_calls;
