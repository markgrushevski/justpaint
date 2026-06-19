-- name: CreateDrawing :one
insert into drawings (owner_id, match_id, doc_version, width, height, document, thumbnail_url)
values ($1, $2, $3, $4, $5, $6, $7)
returning *;

-- name: GetDrawing :one
select * from drawings
where id = $1 and owner_id = $2;

-- name: UpdateDrawing :one
update drawings
set doc_version   = $3,
    width         = $4,
    height        = $5,
    document      = $6,
    thumbnail_url = $7,
    updated_at    = now()
where id = $1 and owner_id = $2
returning *;

-- name: DeleteDrawing :execrows
delete from drawings
where id = $1 and owner_id = $2;

-- name: ListDrawings :many
-- Metadata only (no document body) — keeps the list cheap (docs/API.md §7).
-- Keyset pagination on (created_at desc, id desc); kind filters free/duel/all.
select id, owner_id, match_id, doc_version, width, height, thumbnail_url, created_at, updated_at
from drawings
where owner_id = sqlc.arg('owner_id')
  and (
    sqlc.arg('kind')::text = 'all'
    or (sqlc.arg('kind')::text = 'free' and match_id is null)
    or (sqlc.arg('kind')::text = 'duel' and match_id is not null)
  )
  and (
    sqlc.narg('cursor_created_at')::timestamptz is null
    or (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
order by created_at desc, id desc
limit sqlc.arg('page_limit');
