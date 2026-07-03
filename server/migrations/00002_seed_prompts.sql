-- +goose Up

-- Seed the shared drawing targets a duel is judged against (docs/GAME.md §5).
-- Selection is random among active=true; retiring a prompt is `active = false`
-- (never a delete — matches keep their pinned prompt_id forever). This seed is
-- the v1 starter set; more can be inserted later without a migration.
insert into prompts (text) values
    ('a fox riding a bicycle'),
    ('a lighthouse in a storm'),
    ('a cat wearing a top hat'),
    ('a robot watering a plant'),
    ('a hot air balloon over mountains'),
    ('a dragon reading a book'),
    ('a slice of pizza floating in space'),
    ('an octopus playing the drums'),
    ('a snowman on a sunny beach'),
    ('a treehouse at sunset'),
    ('a wizard brewing coffee'),
    ('a submarine full of fish'),
    ('a bear juggling apples'),
    ('a castle made of candy'),
    ('a penguin on a skateboard'),
    ('a mushroom house in a forest'),
    ('a whale flying with balloons'),
    ('a campfire under the stars'),
    ('a frog wearing sunglasses'),
    ('a steaming bowl of ramen'),
    ('a vintage car on a coastal road'),
    ('an owl delivering a letter'),
    ('a cactus in a cowboy hat'),
    ('a jellyfish disco party');

-- +goose Down

-- Remove only the seeded rows that no match has pinned — a pinned prompt is
-- referenced by matches.prompt_id (FK), and history is never rewritten. In a
-- clean dev DB this deletes the whole starter set.
delete from prompts p
where p.text in (
    'a fox riding a bicycle',
    'a lighthouse in a storm',
    'a cat wearing a top hat',
    'a robot watering a plant',
    'a hot air balloon over mountains',
    'a dragon reading a book',
    'a slice of pizza floating in space',
    'an octopus playing the drums',
    'a snowman on a sunny beach',
    'a treehouse at sunset',
    'a wizard brewing coffee',
    'a submarine full of fish',
    'a bear juggling apples',
    'a castle made of candy',
    'a penguin on a skateboard',
    'a mushroom house in a forest',
    'a whale flying with balloons',
    'a campfire under the stars',
    'a frog wearing sunglasses',
    'a steaming bowl of ramen',
    'a vintage car on a coastal road',
    'an owl delivering a letter',
    'a cactus in a cowboy hat',
    'a jellyfish disco party'
)
and not exists (select 1 from matches m where m.prompt_id = p.id);
