-- +goose Up

-- Drawings get a user-editable display name (docs/API.md §7). This is drawing
-- METADATA, not part of the vector document — the document validators (TS + Go)
-- never see it, so the document contract is untouched.
-- `text` (not varchar(64)): the 64-rune cap is an API write-edge rule like the
-- other field caps (docs/API.md §6), and a byte-typed column would double-enforce
-- it subtly differently (bytes vs runes). NOT NULL + default keeps every
-- pre-existing and name-less row presentable ('new art') without a nullable
-- branch in the API.
alter table drawings add column name text not null default 'new art';

-- +goose Down

alter table drawings drop column name;
