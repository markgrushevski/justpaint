-- +goose Up
-- When a player last showed they were still there (docs/GAME.md §4.1). Matchmaking
-- seats a joiner only against a creator seen recently, and the sweeper abandons an open
-- match whose creator went quiet.
alter table match_players add column seen_at timestamptz not null default now();

-- +goose Down
alter table match_players drop column seen_at;
