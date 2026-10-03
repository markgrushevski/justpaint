<script lang="ts" setup>
/**
 * The mode switcher every screen carries top-left: the wordmark and Draw / Practice /
 * Duel as links, the route's mode marked (`route.meta.mode`). Narrower than
 * `collapseBelow` it folds into one menu button, so a phone, or a game screen whose
 * prompt sits top-center, keeps its top row.
 */
import { computed, onBeforeUnmount, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { OriButton, OriIcon, OriMenu } from '@oriui/vue'
import { icons } from '@core'
import IslandSurface from './ui/IslandSurface.vue'
import type { AppMode } from '../router'

const props = withDefaults(defineProps<{ collapseBelow?: number }>(), { collapseBelow: 600 })

const MODES: { mode: AppMode; label: string; to: string }[] = [
    { mode: 'draw', label: 'Draw', to: '/draw' },
    { mode: 'practice', label: 'Practice', to: '/practice' },
    { mode: 'duel', label: 'Duel', to: '/play' }
]

const route = useRoute()
const router = useRouter()
const current = computed(() => route.meta.mode ?? 'draw')
const currentLabel = computed(() => MODES.find((m) => m.mode === current.value)?.label ?? 'Draw')
const menuItems = MODES.map((m) => ({ value: m.mode, label: m.label }))

const query = window.matchMedia(`(width < ${props.collapseBelow}px)`)
const collapsed = ref(query.matches)
const onQuery = (e: MediaQueryListEvent) => (collapsed.value = e.matches)
query.addEventListener('change', onQuery)
onBeforeUnmount(() => query.removeEventListener('change', onQuery))

function go(mode: string) {
    const target = MODES.find((m) => m.mode === mode)
    // By path, not mode: the gallery is in Draw's mode but isn't /draw.
    if (target && route.path !== target.to) router.push(target.to)
}
</script>

<template>
    <IslandSurface as="nav" class="mode-nav" aria-label="Modes" elevation="md">
        <OriMenu v-if="collapsed" :items="menuItems" placement="bottom-start" @select="go">
            <template #trigger="{ props: trigger }">
                <OriButton
                    v-bind="trigger"
                    class="mode-nav__trigger"
                    variant="text"
                    color="surface"
                    radius="md"
                    size="sm"
                    :aria-label="`Mode: ${currentLabel}`"
                >
                    <span class="mode-nav__trigger-label">{{ currentLabel }}</span>
                    <OriIcon :icon="icons.mdiChevronDown" />
                </OriButton>
            </template>
        </OriMenu>

        <template v-else>
            <span class="mode-nav__brand">just<span class="mode-nav__brand-accent">paint</span></span>
            <ul class="mode-nav__list">
                <li
                    v-for="m in MODES"
                    :key="m.mode"
                    class="mode-nav__item"
                    :class="{ 'mode-nav__item--current': m.mode === current }"
                >
                    <!-- RouterLink sets aria-current="page" on the exact route only, so the
                         gallery marks Draw visually without claiming to be /draw. -->
                    <OriButton
                        :as="RouterLink"
                        :to="m.to"
                        :label="m.label"
                        variant="text"
                        :color="m.mode === current ? 'primary' : 'surface'"
                        radius="md"
                        size="sm"
                    />
                </li>
            </ul>
        </template>
    </IslandSurface>
</template>

<style scoped>
/* The page background, not the surface: the orange wordmark clears the large-text
   3:1 bar on the background only (scripts/check-contrast.mjs). */
.mode-nav {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.5rem);

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
    padding-left: var(--ori-size-gap_lg, 0.75rem);

    background-color: var(--ori-color-background);
    pointer-events: auto;
}

.mode-nav:has(.mode-nav__trigger) {
    padding-left: var(--ori-size-gap_xs, 0.125rem);
}

.mode-nav__brand {
    font-weight: 800;
    font-size: 1.2rem;
    letter-spacing: -0.02em;
    white-space: nowrap;
    color: var(--ori-color-on-background);
}

.mode-nav__brand-accent {
    color: var(--ori-color-primary);
}

.mode-nav__list {
    list-style: none;
    margin: 0;
    padding: 0;

    display: flex;
    gap: var(--ori-size-gap_xs, 0.125rem);
}

.mode-nav__item {
    position: relative;
}

/* The current mode's underline draws in from the center on arrival. */
.mode-nav__item--current::after {
    content: '';
    position: absolute;
    left: 25%;
    right: 25%;
    bottom: 1px;

    height: 2px;
    border-radius: 1px;
    background-color: var(--ori-color-primary);

    animation: mode-nav-underline 260ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

@keyframes mode-nav-underline {
    from {
        transform: scaleX(0);
    }
}

.mode-nav__trigger-label {
    font-weight: 700;
}

@media (prefers-reduced-motion: reduce) {
    .mode-nav__item--current::after {
        animation: none;
    }
}
</style>
