-- +goose Up

-- Drawings get a user-editable display name (docs/API.md §7) — metadata, not
-- part of the vector document, so it never reaches the document validators.
-- `text`, not varchar(64): the 64-rune cap is an API write-edge rule (docs/API.md
-- §6), and a byte-typed column would enforce it differently (bytes vs runes).
alter table drawings add column name text not null default 'new art';

-- +goose Down

alter table drawings drop column name;
