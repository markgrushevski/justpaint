-- name: CreateDrawing :one
-- `name` is user-editable metadata, NOT part of the vector document (the
-- validators never see it — docs/API.md §7). A null name takes the default
-- 'new art' (same value as the column default) via coalesce, so callers with
-- no name concept — game submit — simply pass nil.
insert into drawings (owner_id, match_id, name, doc_version, width, height, document, thumbnail_url)
values (sqlc.arg('owner_id'),
        sqlc.narg('match_id'),
        coalesce(sqlc.narg('name')::text, 'new art'),
        sqlc.arg('doc_version'),
        sqlc.arg('width'),
        sqlc.arg('height'),
        sqlc.arg('document'),
        sqlc.narg('thumbnail_url'))
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
-- `name` uses coalesce so a null (name absent/blank in the request) KEEPS the
-- existing name instead of clobbering it — an update is a document replace, not
-- necessarily a rename (docs/API.md §7).
update drawings
set name        = coalesce(sqlc.narg('name')::text, name),
    doc_version = sqlc.arg('doc_version'),
    width       = sqlc.arg('width'),
    height      = sqlc.arg('height'),
    document    = sqlc.arg('document'),
    updated_at  = now()
where id = sqlc.arg('id') and owner_id = sqlc.arg('owner_id') and match_id is null
returning *;

-- name: DeleteDrawing :execrows
-- Same immutability guard as UpdateDrawing: a submitted duel drawing cannot be
-- deleted via CRUD (it is referenced by match_players.drawing_id — docs/API.md §7).
delete from drawings
where id = $1 and owner_id = $2 and match_id is null;

-- name: ListDrawings :many
-- Metadata only (no document body) — keeps the list cheap (docs/API.md §7).
-- Keyset pagination on (created_at desc, id desc); kind filters free/duel/all.
select id, owner_id, match_id, name, doc_version, width, height, thumbnail_url, created_at, updated_at
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
