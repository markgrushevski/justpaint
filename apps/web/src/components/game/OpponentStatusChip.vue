<script lang="ts">
/**
 * The opponent's live state within a round. Deliberately coarse — the client is
 * only ever told THAT the opponent acted, never WHAT they drew (GAME.md §4.2
 * visibility rule); the opponent canvas is revealed only on the result screen.
 */
export type OpponentStatus = 'drawing' | 'submitted' | 'judging'
</script>

<script lang="ts" setup>
/**
 * OpponentStatusChip — a top-left readout of who you're dueling and where they
 * are in the round: an avatar, their display name, and a coloured status dot.
 *
 * IDENTITY RULE (GAME.md / DECISIONS): show the display name (or the positional
 * "Player 2"), NEVER the opponent's login. PlayView passes `name` already
 * resolved to a safe label; this component never sees a login.
 */
import { computed } from 'vue'
import { OriAvatar } from '@oriui/vue'

const props = defineProps<{
    /** A safe display label — a nickname or "Player 2", NEVER a login. */
    name: string
    /** Where the opponent is in the round (drives the status dot + text). */
    status: OpponentStatus
}>()

const STATUS_LABEL: Record<OpponentStatus, string> = {
    drawing: 'drawing',
    submitted: 'submitted',
    judging: 'judging'
}
const label = computed(() => STATUS_LABEL[props.status])
/** submitted is a settled state (solid dot); the others are in-progress (pulse + ellipsis). */
const inProgress = computed(() => props.status !== 'submitted')
</script>

<template>
    <div class="opp jp-float">
        <OriAvatar class="opp__avatar" :text="name" color="secondary" size="sm" />
        <div class="opp__who">
            <span class="opp__name">{{ name }}</span>
            <span class="opp__status" :class="`opp__status--${status}`">
                <span class="opp__dot" :class="{ 'opp__dot--live': inProgress }" aria-hidden="true"></span>
                <span class="opp__status-text">{{ label }}<template v-if="inProgress">…</template></span>
            </span>
        </div>
    </div>
</template>

<style scoped>
.opp {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
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
    opacity: 0.85;
}

.opp__status-text {
    font-variant-numeric: tabular-nums;
}

.opp__dot {
    flex: none;

    width: 0.5rem;
    height: 0.5rem;

    border-radius: 50%;
    background-color: var(--dot-color, var(--ori-color-outline));
}

.opp__status--drawing {
    --dot-color: var(--ori-color-warn);
}

.opp__status--submitted {
    --dot-color: var(--ori-color-success);
}

.opp__status--judging {
    --dot-color: var(--ori-color-info);
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
