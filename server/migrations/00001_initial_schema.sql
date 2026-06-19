-- +goose Up

-- citext = case-insensitive text, so logins compare case-insensitively
-- (Ada@x.com == ada@x.com) without lower()-ing everywhere.
create extension if not exists citext;

-- users: identity is a single `login` (email OR nickname) + bcrypt password.
-- display_name optional. rating is the live Elo (docs/GAME.md §8).
create table users (
    id            uuid primary key default gen_random_uuid(),
    login         citext      not null unique,
    password_hash text        not null,
    display_name  text,
    rating        int         not null default 1200,
    created_at    timestamptz not null default now(),
    updated_at    timestamptz not null default now()
);

-- prompts: the shared targets a duel is judged against (docs/GAME.md §5).
create table prompts (
    id         uuid        primary key default gen_random_uuid(),
    text       text        not null,
    active     boolean     not null default true,
    created_at timestamptz not null default now()
);

-- matches: one duel; pins one prompt; status drives the lifecycle (docs/GAME.md §3).
-- winner_player_id is nullable: null = tie/undecided (ties are allowed).
create table matches (
    id               uuid        primary key default gen_random_uuid(),
    prompt_id        uuid        not null references prompts (id),
    mode             text        not null default 'async' check (mode in ('async', 'live')),
    status           text        not null default 'open'  check (status in ('open', 'drawing', 'judging', 'done', 'abandoned')),
    winner_player_id uuid        references users (id),
    judge_reason     text,
    created_at       timestamptz not null default now(),
    updated_at       timestamptz not null default now()
);

-- drawings: the vector document stored as jsonb (docs/DOCUMENT-FORMAT.md §7).
-- Queried fields (owner_id, doc_version, width, height) are promoted to columns;
-- the picture itself stays opaque in `document`. match_id null = a free /draw save.
create table drawings (
    id            uuid        primary key default gen_random_uuid(),
    owner_id      uuid        not null references users (id),
    match_id      uuid        references matches (id),
    doc_version   int         not null,
    width         int         not null,
    height        int         not null,
    document      jsonb       not null,
    thumbnail_url text,
    created_at    timestamptz not null default now(),
    updated_at    timestamptz not null default now()
);

-- Keyset (cursor) pagination for "my drawings, newest first" (docs/API.md §7):
-- ordering by (created_at desc, id desc) lets a WHERE (created_at, id) < (cursor)
-- seek the next page in O(log n), unlike OFFSET which re-scans skipped rows.
create index drawings_owner_created_idx on drawings (owner_id, created_at desc, id desc);

-- match_players: the join, two rows per 1v1. The composite PK (match_id, user_id)
-- enforces one slot per player and generalizes to teams/tournaments later.
create table match_players (
    match_id      uuid not null references matches (id),
    user_id       uuid not null references users (id),
    drawing_id    uuid references drawings (id),
    score         double precision,
    rating_before int,
    rating_after  int,
    submitted_at  timestamptz,
    primary key (match_id, user_id)
);

create index match_players_user_idx on match_players (user_id);

-- +goose Down

drop table if exists match_players;
drop table if exists drawings;
drop table if exists matches;
drop table if exists prompts;
drop table if exists users;
-- citext extension intentionally left installed.
