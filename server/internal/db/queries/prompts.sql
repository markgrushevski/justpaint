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
