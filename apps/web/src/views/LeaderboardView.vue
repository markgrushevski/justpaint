<script lang="ts" setup>
/**
 * LeaderboardView — the ranked-players ladder (`/leaderboard`). This is the app's
 * FIRST plain (non-editor) page: it deliberately does NOT mount EditorShell (a
 * Konva canvas shell, wrong for a table), just a centered scrollable container
 * with one OriSurface island on the desk. Purely a cached READ — `useLeaderboard`
 * (docs/API.md §11) owns the fetch/cache; this view only renders the
 * pending / error / empty / data states and highlights the signed-in player's own
 * row. `displayName` is nullable and never a login (GAME.md §4.2), so a safe
 * fallback label is used for anonymous players.
 */
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { OriAvatar, OriBadge, OriButton, OriSkeleton, OriSurface } from '@oriui/vue'
import { icons, toApiError, useLeaderboard, useSessionStore } from '@core'
import type { LeaderboardEntry } from '@core'

/** Top-N shown. Fixed for the page's lifetime (a plain query key is enough). */
const LIMIT = 20

const session = useSessionStore()
const { data, isPending, isError, error } = useLeaderboard(LIMIT)

const entries = computed<LeaderboardEntry[]>(() => data.value?.leaderboard ?? [])
const currentUserId = computed(() => session.user?.id ?? null)
const errorMessage = computed(
    () => toApiError(error.value)?.message ?? 'Could not load the leaderboard. Try again later.'
)

/** Safe display label — `displayName` is nullable server-side; never a login. */
function nameFor(entry: LeaderboardEntry): string {
    return entry.displayName ?? 'Anonymous player'
}
</script>

<template>
    <main class="lb" aria-labelledby="lb-title">
        <OriSurface class="lb__panel">
            <header class="lb__header">
                <div class="lb__heading">
                    <h1 id="lb-title" class="lb__title">Leaderboard</h1>
                    <p class="lb__subtitle">Top players by rating</p>
                </div>
                <OriButton
                    class="lb__back"
                    :as="RouterLink"
                    to="/draw"
                    text="Back to drawing"
                    variant="outline"
                    color="surface"
                    radius="md"
                    :icon="icons.mdiPencil"
                    icon-position="left"
                />
            </header>

            <!-- Data + loading render as the semantic table; error / empty replace it. -->
            <table v-if="isPending || entries.length" class="lb__table">
                <caption class="lb__sr-only">
                    Ranked players by rating, highest first.
                </caption>
                <thead>
                    <tr>
                        <th scope="col" class="lb__th lb__th--rank">Rank</th>
                        <th scope="col" class="lb__th lb__th--player">Player</th>
                        <th scope="col" class="lb__th lb__th--rating">Rating</th>
                        <th scope="col" class="lb__th lb__th--record">W&ndash;L</th>
                    </tr>
                </thead>
                <tbody>
                    <!-- Pending: shimmer rows (hidden from AT — no real data yet). -->
                    <template v-if="isPending">
                        <tr v-for="n in 8" :key="'skeleton-' + n" class="lb__row" aria-hidden="true">
                            <td class="lb__td lb__td--rank"><OriSkeleton class="lb__skel lb__skel--rank" /></td>
                            <td class="lb__td lb__td--player">
                                <div class="lb__player">
                                    <OriSkeleton class="lb__skel lb__skel--avatar" radius="rounded" />
                                    <OriSkeleton class="lb__skel lb__skel--name" />
                                </div>
                            </td>
                            <td class="lb__td lb__td--rating"><OriSkeleton class="lb__skel lb__skel--num" /></td>
                            <td class="lb__td lb__td--record"><OriSkeleton class="lb__skel lb__skel--num" /></td>
                        </tr>
                    </template>
                    <!-- Data. -->
                    <template v-else>
                        <tr
                            v-for="entry in entries"
                            :key="entry.userId"
                            class="lb__row"
                            :class="{ 'lb__row--me': entry.userId === currentUserId }"
                            :aria-current="entry.userId === currentUserId ? 'true' : undefined"
                        >
                            <td class="lb__td lb__td--rank">{{ entry.rank }}</td>
                            <td class="lb__td lb__td--player">
                                <div class="lb__player">
                                    <OriAvatar class="lb__avatar" :text="nameFor(entry)" color="primary" size="sm" />
                                    <span class="lb__name">{{ nameFor(entry) }}</span>
                                    <OriBadge
                                        v-if="entry.userId === currentUserId"
                                        class="lb__you"
                                        content="You"
                                        color="primary"
                                        variant="tonal"
                                        label="This is you"
                                    />
                                </div>
                            </td>
                            <td class="lb__td lb__td--rating">{{ entry.rating }}</td>
                            <td class="lb__td lb__td--record">{{ entry.wins }}&ndash;{{ entry.losses }}</td>
                        </tr>
                    </template>
                </tbody>
            </table>

            <!-- Error: surface the ApiError message (role=alert for AT). -->
            <p v-else-if="isError" class="lb__state lb__state--error" role="alert">{{ errorMessage }}</p>
            <!-- Empty. -->
            <p v-else class="lb__state">No ranked players yet — play a duel.</p>
        </OriSurface>
    </main>
</template>

<style scoped>
.lb {
    /* Fills the shell's remaining height and scrolls internally (the app root is
       overflow:hidden). The desk ground makes the surface island read as a card. */
    flex: 1;
    min-height: 0;
    width: 100%;
    overflow-y: auto;

    display: flex;
    justify-content: center;
    align-items: flex-start;

    padding: clamp(1rem, 4vw, 3rem) 1rem;

    background-color: var(--jp-desk);
}

.lb__panel {
    width: min(760px, 100%);

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_lg, 0.75rem);

    padding: var(--ori-size-gap_xl, 1rem);
}

.lb__header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--ori-size-gap_md, 0.5rem);
}

.lb__heading {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_xs, 0.125rem);
}

.lb__title {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_xl, 1.4rem);
    font-weight: 800;
    letter-spacing: -0.01em;
}

.lb__subtitle {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.875rem);
    /* 0.7 keeps the muted line past WCAG AA on the surface (matches SideMenu). */
    opacity: 0.7;
}

.lb__back {
    flex: none;
}

/* --- table ------------------------------------------------------------- */

.lb__table {
    width: 100%;
    border-collapse: collapse;

    font-size: var(--ori-font-size_sm, 0.9rem);
}

/* Visually-hidden caption — the sighted heading is the <h1>; this names the
   table for assistive tech without duplicating it on screen. */
.lb__sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
    border: 0;
}

.lb__th {
    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);
    /* The header underline uses the neutral outline role at full strength. */
    border-bottom: 1px solid var(--ori-color-outline);

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_xs, 0.75rem);
    font-weight: 700;
    letter-spacing: 0.04em;
    text-align: left;
    text-transform: uppercase;
    /* 0.7 keeps the muted header past WCAG AA (same reasoning as SideMenu). */
    opacity: 0.7;
}

.lb__th--rank,
.lb__td--rank,
.lb__th--rating,
.lb__td--rating,
.lb__th--record,
.lb__td--record {
    text-align: right;
}

.lb__th--rank,
.lb__td--rank {
    width: 3.5rem;
}

.lb__th--rating,
.lb__td--rating,
.lb__th--record,
.lb__td--record {
    width: 5rem;
}

.lb__row {
    /* Inter-row separators derived from the neutral outline role (design-system
       §1 — a neutral structural token, not a banned brand-role re-mix); softened
       so 20 rows don't read as a heavy grid. */
    border-bottom: 1px solid color-mix(in srgb, var(--ori-color-outline) 45%, transparent);
}

.lb__row--me {
    /* Neutral tint (on-surface, not a brand role) marking the signed-in player's
       own row — pairs with aria-current="true" for assistive tech. */
    background-color: color-mix(in srgb, var(--ori-color-on-surface) 8%, transparent);
}

.lb__td {
    padding: var(--ori-size-gap_sm, 0.35rem) var(--ori-size-gap_md, 0.5rem);
    vertical-align: middle;

    color: var(--ori-color-on-surface);
}

.lb__td--rank {
    font-weight: 700;
    font-variant-numeric: tabular-nums;
    opacity: 0.85;
}

.lb__td--rating {
    font-weight: 800;
    font-variant-numeric: tabular-nums;
}

.lb__td--record {
    font-variant-numeric: tabular-nums;
    opacity: 0.85;
}

.lb__player {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.4rem);
    min-width: 0;
}

.lb__avatar,
.lb__you {
    flex: none;
}

.lb__name {
    overflow: hidden;

    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
}

/* --- states ------------------------------------------------------------ */

.lb__state {
    margin: 0;
    padding: var(--ori-size-gap_lg, 0.75rem) var(--ori-size-gap_md, 0.5rem);

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.9rem);
    text-align: center;
    opacity: 0.75;
}

.lb__state--error {
    /* oriui's AA-guaranteed danger text tone (falls back to the role token). */
    color: var(--ori-color-danger-text, var(--ori-color-danger));
    opacity: 1;
}

/* --- loading shimmer --------------------------------------------------- */

.lb__skel {
    display: block;
    height: 0.85rem;

    border-radius: var(--ori-size-radius_sm, 4px);
}

.lb__skel--rank,
.lb__skel--num {
    width: 2.5rem;
    /* right-align inside the numeric cells */
    margin-left: auto;
}

.lb__skel--avatar {
    flex: none;
    width: 1.75rem;
    height: 1.75rem;
}

.lb__skel--name {
    width: min(11rem, 45vw);
    height: 0.9rem;
}
</style>
