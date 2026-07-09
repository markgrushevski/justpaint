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
import { OriAvatar, OriButton, OriField, OriIcon, OriInput, OriSelect, OriSwitch, OriTabs } from '@oriui/vue'
import { icons, toApiError, useSessionStore, useThemeStore } from '@core'
import type { ThemeMode } from '@core'
import ToolIcon from './icons/ToolIcon.vue'
import type { IconName } from './icons/ToolIcon.vue'

const props = defineProps<{
    open: boolean
    busy: boolean
    canRename: boolean
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

// --------------------------------------------------------------- appearance

// Theme lives here now (moved out of the /draw actions island). A 3-way
// segmented control bound to the theme store's writable `mode`: assigning
// applies + persists through useThemeStore (the single source of truth). oriui
// ships no segmented primitive, so this is a small hand-rolled segment group.
const THEME_OPTIONS: { value: ThemeMode; label: string; icon: IconName }[] = [
    { value: 'light', label: 'Light', icon: 'sun' },
    { value: 'dark', label: 'Dark', icon: 'moon' },
    { value: 'auto', label: 'Auto', icon: 'monitor' }
]

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

// ------------------------------------------------------------- auth section

const authMode = ref<'login' | 'register'>('login')
const loginId = ref('')
const password = ref('')
const error = ref<string | null>(null)
const authBusy = ref(false)

const AUTH_TABS = [
    { value: 'login', label: 'Log in' },
    { value: 'register', label: 'Register' }
]

// OriTabs models `string | number | undefined` — bridge the narrowed union.
const authTab = computed<string | number | undefined>({
    get: () => authMode.value,
    set: (v) => (authMode.value = v === 'register' ? 'register' : 'login')
})

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
            error.value = null
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

async function submit() {
    if (authBusy.value) return
    const id = loginId.value.trim()
    if (!id || !password.value) {
        error.value = 'Enter a login and password.'
        return
    }
    error.value = null
    authBusy.value = true
    try {
        if (authMode.value === 'register') await session.register(id, password.value)
        else await session.login(id, password.value)
        password.value = ''
    } catch (err) {
        error.value = toApiError(err)?.message ?? 'Auth failed (is the server running?).'
    } finally {
        authBusy.value = false
    }
}

async function logout() {
    await session.logout()
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
                    v-if="props.canRename"
                    class="menu__title menu__title--editable"
                    contenteditable="true"
                    role="textbox"
                    aria-label="Drawing name"
                    spellcheck="false"
                    @blur="commitTitle"
                    @keydown.enter="onTitleEnter"
                    >{{ displayTitle }}</span
                >
                <span v-else class="menu__title" title="Sign in to rename">{{ displayTitle }}</span>
                <OriIcon v-if="props.canRename" :icon="icons.mdiRename" class="menu__title-pencil" />
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
                        :disabled="!session.isLoggedIn"
                        @click="fileSave"
                    />
                    <OriButton
                        text="Load"
                        variant="outline"
                        radius="md"
                        fluid
                        :icon="icons.mdiCloudDownloadOutline"
                        :loading="props.busy"
                        :disabled="!session.isLoggedIn"
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
                <p v-if="!session.isLoggedIn" class="menu__hint">Sign in to save &amp; load</p>
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
                <div class="menu__theme" role="group" aria-label="Theme">
                    <button
                        v-for="opt in THEME_OPTIONS"
                        :key="opt.value"
                        type="button"
                        class="menu__theme-btn"
                        :class="{ 'menu__theme-btn--active': theme.mode === opt.value }"
                        :aria-pressed="theme.mode === opt.value"
                        @click="theme.mode = opt.value"
                    >
                        <ToolIcon :name="opt.icon" />
                        <span class="menu__theme-label">{{ opt.label }}</span>
                    </button>
                </div>
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
                <OriButton text="Log out" variant="outline" radius="md" :icon="icons.mdiLogout" @click="logout" />
            </section>

            <!-- Auth (anonymous) -->
            <section v-else class="menu__section menu__section--bottom" aria-label="Sign in">
                <OriTabs v-model="authTab" :tabs="AUTH_TABS">
                    <div class="menu__form">
                        <OriField label="Login">
                            <OriInput
                                v-model="loginId"
                                placeholder="email or nickname"
                                autocomplete="username"
                                @keyup.enter="submit"
                            />
                        </OriField>
                        <OriField
                            label="Password"
                            :error="error ?? undefined"
                            hint="Sign in to save and load your drawings."
                        >
                            <OriInput
                                v-model="password"
                                type="password"
                                placeholder="password"
                                :autocomplete="authMode === 'register' ? 'new-password' : 'current-password'"
                                @keyup.enter="submit"
                            />
                        </OriField>
                        <OriButton
                            :text="authMode === 'register' ? 'Create account' : 'Log in'"
                            variant="fill"
                            radius="md"
                            fluid
                            :loading="authBusy"
                            @click="submit"
                        />
                    </div>
                </OriTabs>
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

.menu__hint {
    margin: var(--ori-size-gap_xs, 0.25rem) 0 0;
    font-size: var(--ori-font-size_xs, 0.75rem);
    opacity: 0.7;
}

/* Theme segmented control — three joined segments (oriui ships no segmented
   primitive). The active segment fills primary, mirroring the accent Save chip
   in the /draw shell; unlayered so it needs no !important. */
.menu__theme {
    display: flex;

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
    border-radius: var(--ori-size-radius_md, 8px);
    overflow: hidden;
}

.menu__theme-btn {
    flex: 1;

    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);

    border: none;
    background: transparent;
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    cursor: pointer;
}

/* Hairline dividers between segments (not before the first). */
.menu__theme-btn + .menu__theme-btn {
    border-left: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
}

.menu__theme-btn:focus-visible {
    outline: 2px solid var(--ori-color-primary);
    outline-offset: -2px;
}

.menu__theme-btn:hover:not(.menu__theme-btn--active) {
    background-color: var(--jp-hover-bg, color-mix(in srgb, var(--ori-color-primary) 12%, transparent));
}

.menu__theme-btn--active {
    background-color: var(--ori-color-primary);
    color: var(--ori-color-on-primary);
}

.menu__theme-label {
    font-weight: 600;
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

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
    border-radius: var(--ori-size-radius_md, 8px);
    background-color: var(--ori-color-background);

    font-size: var(--ori-font-size_sm, 0.9rem);
}

.menu__form {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.5rem);
    padding-top: var(--ori-size-gap_md, 0.5rem);
}
</style>
