-- name: PickRandomActivePrompt :one
-- Selection (v1) is random among active prompts (docs/GAME.md §5). `order by
-- random()` is fine at seed scale (dozens of rows); revisit for a large pool.
select * from prompts
where active
order by random()
limit 1;

-- name: GetPromptByID :one
select * from prompts
where id = $1;

-- name: GetActivePromptByID :one
-- The prompt a practice run claims to be answering. `active` is part of the
-- lookup, not a field the caller checks afterwards: a deactivated prompt must read
-- as "no such prompt" (→ 404), the same answer a made-up id gets, so retiring a
-- prompt cannot be detected by trying to draw for it. A duel pins its prompt
-- server-side and so has no equivalent — only practice takes an id from a client.
select * from prompts
where id = $1 and active;
