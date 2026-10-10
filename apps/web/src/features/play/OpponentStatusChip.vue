<script lang="ts" setup>
/**
 * OpponentStatusChip — who you're dueling and where they are in the round: an avatar,
 * their display name, and a status dot with its word. Coarse on purpose: the client
 * only learns that the opponent acted, never what they drew (docs/GAME.md §4.2).
 *
 * Identity rule (docs/GAME.md §4.2): a display name or the positional "Player 2",
 * never a login; PlayView passes the label already resolved.
 */
import { OriAvatar } from '@oriui/vue'
import IslandSurface from '../../components/ui/IslandSurface.vue'
import type { OpponentStatus } from './duel'

defineProps<{
    /** A safe display label — a nickname or "Player 2", never a login. */
    name: string
    status: OpponentStatus
}>()
</script>

<template>
    <IslandSurface class="opp" elevation="md">
        <OriAvatar class="opp__avatar" :name="name" color="secondary" size="sm" />
        <div class="opp__who">
            <span class="opp__name">{{ name }}</span>
            <span class="opp__status" :class="`opp__status--${status}`">
                <span class="opp__dot" :class="{ 'opp__dot--live': status === 'drawing' }" aria-hidden="true"></span>
                <span class="opp__status-text">{{ status }}<template v-if="status === 'drawing'">…</template></span>
            </span>
        </div>
    </IslandSurface>
</template>

<style scoped>
.opp {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.5rem);

    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);
    max-width: 60vw;
}

.opp__avatar {
    flex: none;
}

.opp__who {
    display: flex;
    flex-direction: column;
    min-width: 0;
    line-height: 1.15;
}

.opp__name {
    overflow: hidden;

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.opp__status {
    display: flex;
    align-items: center;
    gap: 0.3rem;

    font-size: var(--ori-font-size_xs, 0.75rem);
    /* 0.85 keeps the tiny status line legible past WCAG AA on the surface. */
    opacity: var(--jp-dim, 0.85);
}

.opp__status-text {
    font-variant-numeric: tabular-nums;
}

.opp__dot {
    flex: none;

    width: 0.5rem;
    height: 0.5rem;

    border-radius: 50%;
    background-color: var(--dot-color, var(--jp-color-outline));
}

.opp__status--drawing {
    --dot-color: var(--ori-color-warning);
}

.opp__status--submitted {
    --dot-color: var(--ori-color-success);
}

.opp__dot--live {
    animation: opp-pulse 1.1s ease-in-out infinite;
}

@keyframes opp-pulse {
    50% {
        opacity: 0.35;
        transform: scale(0.85);
    }
}

@media (prefers-reduced-motion: reduce) {
    .opp__dot--live {
        animation: none;
    }
}
</style>
