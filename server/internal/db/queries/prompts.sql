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
-- lookup, not checked afterwards, so a retired prompt reads as "no such prompt"
-- like a made-up id (docs/API.md §12). Only practice takes a prompt id from a
-- client; a duel pins its own server-side.
select * from prompts
where id = $1 and active;
