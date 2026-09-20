<script lang="ts" setup>
/**
 * The right-side slide-in menu — the legacy NON-MODAL pattern (owner's spec,
 * 2026-07-07): always mounted, slides in from the right over the canvas with
 * no backdrop and no focus trap, so the canvas stays interactive behind it.
 * Toggled from DrawView (the toggler lives there, not here). Holds the
 * drawing title (inline rename), copy actions, file actions, canvas settings,
 * and — at the bottom, since unregistered users are the /draw priority —
 * auth (login ⇄ register) / the profile.
 */
import { computed, nextTick, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { OriAvatar, OriButton, OriIcon, OriInput, OriSelect, OriSwitch } from '@oriui/vue'
import { icons, useAuthGate, useSessionStore, useThemeStore } from '@core'
import type { ThemeMode } from '@core'
import SegmentedControl from './ui/SegmentedControl.vue'
import type { IconName } from './icons/ToolIcon.vue'

const props = defineProps<{
    open: boolean
    busy: boolean
    title: string
    backdropGrid: boolean
    canvasWidth: number
    canvasHeight: number
}>()
const emit = defineEmits<{
    close: []
    newDrawing: []
    load: []
    save: []
    exportPng: []
    copyText: []
    copyImage: []
    rename: [name: string]
    toggleGrid: [on: boolean]
    applyCanvasSize: [w: number, h: number]
}>()

const session = useSessionStore()
const theme = useThemeStore()
const gate = useAuthGate()

// --------------------------------------------------------------- appearance

// Theme lives here now (moved out of the /draw actions island). The reusable
// SegmentedControl (ui/) binds to the theme store's writable `mode`: assigning
// applies + persists through useThemeStore (the single source of truth).
const THEME_OPTIONS: { value: ThemeMode; label: string; icon: IconName }[] = [
    { value: 'light', label: 'Light', icon: 'sun' },
    { value: 'dark', label: 'Dark', icon: 'moon' },
    { value: 'auto', label: 'Auto', icon: 'monitor' }
]

/** SegmentedControl emits a plain string; narrow it back to the theme union. */
function selectTheme(mode: string): void {
    theme.mode = mode as ThemeMode
}

// ---------------------------------------------------------------- title row

const TITLE_MAX = 64
const displayTitle = computed(() => props.title.slice(0, TITLE_MAX))

/** Commit the inline rename: trimmed, capped, only when it actually changed. */
function commitTitle(e: Event) {
    const el = e.target as HTMLElement
    const next = el.innerText.trim().slice(0, TITLE_MAX)
    if (!next) {
        // Emptied out — restore the current title instead of renaming to "".
        el.innerText = displayTitle.value
        return
    }
    if (next !== props.title) emit('rename', next)
}

/** Enter commits (via blur — single commit path) instead of inserting a newline. */
function onTitleEnter(e: KeyboardEvent) {
    e.preventDefault()
    ;(e.target as HTMLElement).blur()
}

// ----------------------------------------------------------- canvas section

// Screen dimensions for the "Screen" preset label — refreshed on open so a
// rotated phone / resized window shows current numbers.
const screenW = ref(window.innerWidth)
const screenH = ref(window.innerHeight)

const sizeChoice = ref<string | number | undefined>('screen')
const sizeOptions = computed(() => [
    { value: 'screen', label: `Screen (${screenW.value} × ${screenH.value})` },
    { value: '1080', label: '1080 × 1080 (duel)' },
    { value: '1920', label: '1920 × 1080' },
    { value: 'custom', label: 'Custom' }
])

// Custom W/H — OriInput models a string; prefilled from the live canvas size.
const customW = ref(String(props.canvasWidth))
const customH = ref(String(props.canvasHeight))

function clampSize(n: number): number {
    if (!Number.isFinite(n)) return 1
    return Math.min(8192, Math.max(1, Math.round(n)))
}

function applySize() {
    let w: number
    let h: number
    switch (sizeChoice.value) {
        case '1080':
            w = 1080
            h = 1080
            break
        case '1920':
            w = 1920
            h = 1080
            break
        case 'custom':
            w = Number.parseFloat(customW.value)
            h = Number.parseFloat(customH.value)
            break
        default:
            w = window.innerWidth
            h = window.innerHeight
    }
    emit('applyCanvasSize', clampSize(w), clampSize(h))
    emit('close')
}

function onToggleGrid(on: boolean | undefined) {
    emit('toggleGrid', on === true)
}

// ---------------------------------------------------- panel focus management

// Reset transient form state and move focus into the panel whenever the menu
// opens — without focus inside, Esc (keydown on the panel tree) never fires.
const panelRef = ref<HTMLElement | null>(null)
// The element focused before the drawer opened (the toggler in DrawView) —
// focus returns here on close so keyboard users aren't dumped on <body>.
const opener = ref<HTMLElement | null>(null)
watch(
    () => props.open,
    async (open) => {
        if (open) {
            screenW.value = window.innerWidth
            screenH.value = window.innerHeight
            customW.value = String(props.canvasWidth)
            customH.value = String(props.canvasHeight)
            opener.value = document.activeElement as HTMLElement | null
            await nextTick()
            panelRef.value?.focus()
        } else {
            opener.value?.focus()
        }
    }
)

async function logout() {
    await session.logout()
}

// The owner wants the bulky inline auth form out of the drawer (2026-09-18) —
// hand off to the shared modal instead. Close the drawer first: AuthDialog is a
// true modal with its own backdrop, so leaving the drawer's Save/Load/Canvas/
// Appearance sections slid out behind it would just double up on chrome.
const signIn = () => {
    emit('close')
    void gate.ensure()
}

// File actions: emit the action, then close the drawer (the action runs in the
// host view while the menu slides away).
const fileNew = () => {
    emit('newDrawing')
    emit('close')
}
const fileLoad = () => {
    emit('load')
    emit('close')
}
const fileSave = () => {
    emit('save')
    emit('close')
}
const fileExport = () => {
    emit('exportPng')
    emit('close')
}

// Non-modal: no Tab trap — only Esc (from anywhere inside the panel) closes.
function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') emit('close')
}
</script>

<template>
    <Teleport to="body">
        <!-- Always mounted; open/closed is pure transform (the legacy mechanic).
             `inert` while closed keeps the off-screen panel out of the Tab order. -->
        <aside
            ref="panelRef"
            class="menu"
            :class="{ 'menu--open': props.open }"
            role="complementary"
            aria-label="Menu"
            tabindex="-1"
            :inert="!props.open"
            @keydown="onKeydown"
        >
            <!-- Title row: the drawing name (inline rename when allowed) -->
            <header class="menu__title-row">
                <span
                    class="menu__title menu__title--editable"
                    contenteditable="true"
                    role="textbox"
                    aria-label="Drawing name"
                    spellcheck="false"
                    @blur="commitTitle"
                    @keydown.enter="onTitleEnter"
                    >{{ displayTitle }}</span
                >
                <OriIcon :icon="icons.mdiRename" class="menu__title-pencil" />
            </header>

            <!-- Copy row: stays open after copying (legacy behavior) -->
            <div class="menu__copy">
                <OriButton
                    text="Copy as text"
                    variant="tonal"
                    radius="md"
                    :icon="icons.mdiContentCopy"
                    icon-position="left"
                    @click="emit('copyText')"
                />
                <OriButton
                    text="Copy as image"
                    variant="tonal"
                    radius="md"
                    :icon="icons.mdiContentCopy"
                    icon-position="left"
                    @click="emit('copyImage')"
                />
            </div>

            <!-- File actions (the only home for New/Load/Export on phones) -->
            <section class="menu__section" aria-label="File">
                <h2 class="menu__section-title">File</h2>
                <div class="menu__stack">
                    <OriButton
                        text="Save"
                        variant="fill"
                        radius="md"
                        fluid
                        :icon="icons.mdiContentSaveOutline"
                        :loading="props.busy"
                        @click="fileSave"
                    />
                    <OriButton
                        text="Load"
                        variant="outline"
                        radius="md"
                        fluid
                        :icon="icons.mdiCloudDownloadOutline"
                        :loading="props.busy"
                        @click="fileLoad"
                    />
                    <OriButton text="New" variant="outline" radius="md" fluid :icon="icons.mdiPlus" @click="fileNew" />
                    <OriButton
                        text="Export"
                        variant="outline"
                        radius="md"
                        fluid
                        :icon="icons.mdiDownload"
                        @click="fileExport"
                    />
                </div>
            </section>

            <!-- Play — the way OUT of /draw into the game. It sits here, above the
                 fold and outside the profile block, because the drawer is the only
                 durable route to the game: the /draw welcome card carries the same
                 links but disappears on the first stroke and never returns, so a
                 visitor who drew one line used to lose the product's main mode.
                 Duel and practice are open to anonymous visitors (both views gate
                 on mount, so the sign-in prompt arrives with a reason attached);
                 the ladder is not, because GET /api/leaderboard requires a session
                 and an anonymous click would only earn a 401.

                 `tonal`, not `outline`: outline with no colour resolves to the same
                 maroon hairline as Load/New/Export three rows above, so the
                 product's MAIN mode read as "File, part two". DESIGN-SYSTEM §2
                 keeps tonal for grouped mid-emphasis, which is exactly what a
                 navigation trio is — distinct from the file actions without
                 stealing the single `fill` that belongs to Save. -->
            <section class="menu__section" aria-label="Play">
                <h2 class="menu__section-title">Play</h2>
                <div class="menu__stack">
                    <!-- RouterLinks render <a>: the drawer unmounts with /draw on
                         navigation, so none of these needs an explicit close. -->
                    <OriButton
                        :as="RouterLink"
                        to="/play"
                        text="Play a duel"
                        variant="tonal"
                        radius="md"
                        fluid
                        :icon="icons.mdiSwordCross"
                        icon-position="left"
                    />
                    <OriButton
                        :as="RouterLink"
                        to="/practice"
                        text="Practice solo"
                        variant="tonal"
                        radius="md"
                        fluid
                        :icon="icons.target"
                        icon-position="left"
                    />
                    <OriButton
                        v-if="session.isLoggedIn"
                        :as="RouterLink"
                        to="/leaderboard"
                        text="Leaderboard"
                        variant="tonal"
                        radius="md"
                        fluid
                        :icon="icons.podium"
                        icon-position="left"
                    />
                </div>
            </section>

            <!-- Canvas settings -->
            <section class="menu__section" aria-label="Canvas">
                <h2 class="menu__section-title">Canvas</h2>
                <OriSelect v-model="sizeChoice" label="Canvas size" :options="sizeOptions" fluid />
                <div v-if="sizeChoice === 'custom'" class="menu__size-custom">
                    <OriInput v-model="customW" label="W" type="number" min="1" max="8192" fluid />
                    <OriInput v-model="customH" label="H" type="number" min="1" max="8192" fluid />
                </div>
                <OriButton text="Apply size" variant="outline" radius="md" size="sm" @click="applySize" />
                <OriSwitch label="Checkerboard" :model-value="props.backdropGrid" @update:model-value="onToggleGrid" />
            </section>

            <!-- Appearance: theme (moved here from the /draw actions island). The
                 store's writable `mode` applies + persists on assignment. -->
            <section class="menu__section" aria-label="Appearance">
                <h2 class="menu__section-title">Appearance</h2>
                <SegmentedControl
                    :model-value="theme.mode"
                    :options="THEME_OPTIONS"
                    label="Theme"
                    @update:model-value="selectTheme"
                />
            </section>

            <!-- Profile (signed in) — pinned to the bottom: unregistered users
                 are the /draw priority, so auth stays out of the way. -->
            <section v-if="session.isLoggedIn" class="menu__section menu__section--bottom" aria-label="Profile">
                <div class="menu__profile">
                    <OriAvatar :text="session.user?.displayName ?? session.user?.login ?? '?'" color="primary" />
                    <div class="menu__who">
                        <b class="menu__name">{{ session.user?.displayName ?? session.user?.login }}</b>
                        <span class="menu__login">{{ session.user?.login }}</span>
                    </div>
                </div>
                <div class="menu__rating">
                    Rating <b>{{ session.user?.rating }}</b>
                </div>
                <!-- Navigation used to live here, which is why an anonymous visitor
                     saw no game at all. It moved to the Play section above; this
                     block is now only who-you-are and how-to-leave. -->
                <OriButton text="Log out" variant="outline" radius="md" :icon="icons.mdiLogout" @click="logout" />
            </section>

            <!-- Auth (anonymous): one entry point into the shared sign-in modal. -->
            <section v-else class="menu__section menu__section--bottom" aria-label="Sign in">
                <OriButton text="Sign in" variant="outline" radius="md" :icon="icons.mdiLogin" @click="signIn" />
            </section>
        </aside>
    </Teleport>
</template>

<style scoped>
.menu {
    position: fixed;
    top: 0;
    right: 0;
    z-index: 100;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_lg, 0.75rem);

    width: 400px;
    max-width: 100dvw; /* full screen on phones */
    height: 100dvh;
    padding: 12px;
    overflow-y: auto;

    border-left: 1px solid var(--ori-color-primary);
    background-color: var(--ori-color-surface);
    color: var(--ori-color-on-surface);

    /* 101% so the border/shadow can't peek in while closed. */
    transform: translateX(101%);
    transition: transform ease-out 0.25s;
}

.menu--open {
    transform: translateX(0);
    box-shadow: -8px 0 32px rgb(0 0 0 / 18%);
}

.menu__title-row {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
    min-width: 0;
}

.menu__title {
    overflow: hidden;
    min-width: 3ch;
    max-width: 100%;

    font-weight: 700;
    font-size: var(--ori-font-size_lg, 1.15rem);
    text-overflow: ellipsis;
    white-space: nowrap;
    letter-spacing: -0.01em;
}

.menu__title--editable {
    padding: 0 var(--ori-size-gap_xs, 0.125rem);
    border-radius: var(--ori-size-radius_sm, 4px);
    cursor: text;
}

.menu__title--editable:hover {
    background-color: var(--jp-neutral-hover-bg, color-mix(in srgb, var(--ori-color-on-surface) 8%, transparent));
}

.menu__title--editable:focus-visible {
    outline: 2px solid var(--ori-color-primary);
    outline-offset: 1px;
    /* Let long names wrap while editing instead of hiding the caret. */
    text-overflow: clip;
    white-space: normal;
}

.menu__title-pencil {
    flex-shrink: 0;
    opacity: 0.6;
}

.menu__copy {
    display: flex;
    gap: var(--ori-size-gap_md, 0.5rem);
}

.menu__copy > * {
    flex: 1;
}

.menu__section {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.5rem);
}

/* Auth/profile sits at the very bottom — content above stays reachable first. */
.menu__section--bottom {
    margin-top: auto;
}

.menu__section-title {
    margin: 0 0 var(--ori-size-gap_xs, 0.125rem);

    font-size: var(--ori-font-size_xs, 0.75rem);
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    /* 0.7 (not 0.6) keeps the muted header past WCAG AA: the on-surface ink at 0.6 composited only
       4.01:1 on the light surface; 0.7 lifts it to ~5.4:1 (dark theme was already ~6:1). */
    opacity: 0.7;
}

.menu__stack {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.5rem);
}

.menu__size-custom {
    display: flex;
    gap: var(--ori-size-gap_md, 0.5rem);
}

.menu__size-custom > * {
    flex: 1;
}

.menu__profile {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.5rem);
}

.menu__who {
    display: flex;
    flex-direction: column;
    min-width: 0;
}

.menu__name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.menu__login {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;

    font-size: var(--ori-font-size_sm, 0.85rem);
    opacity: 0.7;
}

.menu__rating {
    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);

    border: 1px solid var(--jp-color-outline, rgb(0 0 0 / 12%));
    border-radius: var(--ori-size-radius_md, 8px);
    background-color: var(--ori-color-background);

    font-size: var(--ori-font-size_sm, 0.9rem);
}
</style>
