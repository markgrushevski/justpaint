-- name: CreateDrawing :one
insert into drawings (owner_id, match_id, doc_version, width, height, document, thumbnail_url)
values ($1, $2, $3, $4, $5, $6, $7)
returning *;

-- name: GetDrawing :one
select * from drawings
where id = $1 and owner_id = $2;

-- name: UpdateDrawing :one
-- `match_id is null` makes a submitted duel drawing immutable via CRUD (it is
-- locked once submitted — docs/API.md §7). A match-linked row matches nothing
-- here; the service turns that miss into 409, not a silent 404.
-- thumbnail_url is intentionally NOT set here: it is a server-generated cached-PNG
-- URL (the render worker owns it — DOCUMENT-FORMAT §7), never a client field, so a
-- document CRUD update must leave it untouched rather than clobber it to NULL.
update drawings
set doc_version = $3,
    width       = $4,
    height      = $5,
    document    = $6,
    updated_at  = now()
where id = $1 and owner_id = $2 and match_id is null
returning *;

-- name: DeleteDrawing :execrows
-- Same immutability guard as UpdateDrawing: a submitted duel drawing cannot be
-- deleted via CRUD (it is referenced by match_players.drawing_id — docs/API.md §7).
delete from drawings
where id = $1 and owner_id = $2 and match_id is null;

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
