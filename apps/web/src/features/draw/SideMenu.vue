<script lang="ts" setup>
/**
 * The /draw menu: a non-modal panel under the corner toggler (a right-edge drawer on
 * phones) listing the file actions, then canvas, theme and account. Export and Canvas
 * open as sub-panels in place. No focus trap: the canvas stays live, Esc closes.
 */
import { computed, nextTick, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { OriAvatar, OriButton, OriIcon, OriInput, OriSelect, OriSurface, OriSwitch } from '@oriui/vue'
import { icons, useAuthGate, useSessionStore, useThemeStore } from '@core'
import type { ThemeMode } from '@core'
import MenuRow from '../../components/ui/MenuRow.vue'
import SegmentedControl from '../../components/ui/SegmentedControl.vue'
import type { IconName } from '../../components/icons/ToolIcon.vue'

const props = defineProps<{
    open: boolean
    busy: boolean
    /** The drawing's name once saved, or null while it has never been saved. */
    title: string | null
    backdropGrid: boolean
    canvasWidth: number
    canvasHeight: number
}>()
const emit = defineEmits<{
    close: []
    newDrawing: []
    save: []
    exportPng: []
    copyText: []
    copyImage: []
    shortcuts: []
    toggleGrid: [on: boolean]
    applyCanvasSize: [w: number, h: number]
}>()

const session = useSessionStore()
const theme = useThemeStore()
const gate = useAuthGate()

type View = 'main' | 'export' | 'canvas'
const view = ref<View>('main')

// SegmentedControl binds to the theme store's writable `mode`: assigning applies and
// persists through useThemeStore, the single source of truth.
const THEME_OPTIONS: { value: ThemeMode; label: string; icon: IconName }[] = [
    { value: 'light', label: 'Light', icon: 'sun' },
    { value: 'dark', label: 'Dark', icon: 'moon' },
    { value: 'auto', label: 'Auto', icon: 'monitor' }
]

/** SegmentedControl emits a plain string; narrow it back to the theme union. */
function selectTheme(mode: string): void {
    theme.mode = mode as ThemeMode
}

// Screen dimensions for the "Screen" preset label, refreshed on open so a rotated
// phone or a resized window shows current numbers.
const screenW = ref(window.innerWidth)
const screenH = ref(window.innerHeight)

const sizeChoice = ref<string | number | undefined>('screen')
const sizeOptions = computed(() => [
    { value: 'screen', label: `Screen (${screenW.value} × ${screenH.value})` },
    { value: '1080', label: '1080 × 1080 (duel)' },
    { value: '1920', label: '1920 × 1080' },
    { value: 'custom', label: 'Custom' }
])

// Custom W/H: OriInput models a string; prefilled from the live canvas size.
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

const panelRef = ref<{ $el: HTMLElement } | null>(null)
const panelEl = () => panelRef.value?.$el ?? null

/** Focus the first control of the panel now showing, so arrow-free keyboard use continues there. */
async function focusFirst() {
    await nextTick()
    panelEl()?.querySelector<HTMLElement>('.menu__view button, .menu__view a, .menu__view input')?.focus()
}

function show(next: View) {
    view.value = next
    focusFirst()
}

// Opening resets the panel and moves focus inside, or Esc (keydown on the panel tree)
// never fires; closing returns focus to whatever opened it.
const opener = ref<HTMLElement | null>(null)
watch(
    () => props.open,
    (open) => {
        if (open) {
            view.value = 'main'
            screenW.value = window.innerWidth
            screenH.value = window.innerHeight
            customW.value = String(props.canvasWidth)
            customH.value = String(props.canvasHeight)
            opener.value = document.activeElement as HTMLElement | null
            focusFirst()
        } else {
            opener.value?.focus()
        }
    }
)

// An action runs in the host view while the panel closes.
function run(action: () => void) {
    action()
    emit('close')
}

// Close first: AuthDialog is a true modal with its own backdrop.
function signIn() {
    emit('close')
    gate.ensure()
}

async function logout() {
    await session.logout()
}

// The view's own Esc handler on window would close the whole panel too.
function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return
    e.stopPropagation()
    if (view.value !== 'main') show('main')
    else emit('close')
}
</script>

<template>
    <Teleport to="body">
        <!-- Always mounted; open/closed is pure transform. `inert` while closed keeps the
             hidden panel out of the Tab order. -->
        <OriSurface
            ref="panelRef"
            as="aside"
            class="menu"
            :class="{ 'menu--open': props.open }"
            role="complementary"
            aria-label="Menu"
            tabindex="-1"
            :inert="!props.open"
            @keydown="onKeydown"
        >
            <p class="menu__name" :class="{ 'menu__name--unsaved': !props.title }">
                {{ props.title ?? 'Unsaved drawing' }}
            </p>

            <div v-if="view === 'main'" class="menu__view">
                <MenuRow :icon="icons.mdiPlus" label="New drawing" @click="run(() => emit('newDrawing'))" />
                <MenuRow :icon="icons.mdiImageMultipleOutline" label="My drawings" to="/gallery" />
                <MenuRow
                    :icon="icons.mdiContentSaveOutline"
                    label="Save"
                    hint="Ctrl+S"
                    :disabled="props.busy"
                    @click="run(() => emit('save'))"
                />
                <MenuRow :icon="icons.mdiDownload" label="Export" chevron @click="show('export')" />

                <hr class="menu__rule" />

                <MenuRow :icon="icons.mdiAspectRatio" label="Canvas" chevron @click="show('canvas')" />
                <div class="menu__theme">
                    <OriIcon :icon="icons.mdiThemeLightDark" class="menu__theme-icon" />
                    <SegmentedControl
                        :model-value="theme.mode"
                        :options="THEME_OPTIONS"
                        label="Theme"
                        @update:model-value="selectTheme"
                    />
                </div>
                <MenuRow
                    class="menu__desktop-only"
                    :icon="icons.mdiKeyboard"
                    label="Keyboard shortcuts"
                    hint="?"
                    @click="run(() => emit('shortcuts'))"
                />

                <hr class="menu__rule" />

                <div v-if="session.isLoggedIn" class="menu__profile">
                    <OriAvatar
                        :name="session.user?.displayName ?? session.user?.login ?? '?'"
                        color="primary"
                        size="sm"
                    />
                    <div class="menu__who">
                        <b class="menu__who-name">{{ session.user?.displayName ?? session.user?.login }}</b>
                        <!-- The ladder is the duel's, so /draw reaches it only through the rating.
                             It needs a session: GET /api/leaderboard answers 401 without one. -->
                        <RouterLink
                            class="menu__who-meta menu__rating"
                            to="/leaderboard"
                            :aria-label="`Rating ${session.user?.rating}, open the leaderboard`"
                        >
                            Rating {{ session.user?.rating }}
                            <OriIcon :icon="icons.mdiChevronRight" />
                        </RouterLink>
                    </div>
                    <OriButton label="Log out" variant="outline" radius="md" size="sm" @click="logout" />
                </div>
                <MenuRow v-else :icon="icons.mdiLogin" label="Sign in" @click="signIn" />
            </div>

            <div v-else-if="view === 'export'" class="menu__view">
                <MenuRow :icon="icons.mdiChevronLeft" label="Export" @click="show('main')" />
                <hr class="menu__rule" />
                <MenuRow :icon="icons.mdiDownload" label="Download PNG" @click="run(() => emit('exportPng'))" />
                <!-- Copying leaves the panel open: the next paste is somewhere else. -->
                <MenuRow :icon="icons.mdiContentCopy" label="Copy as image" @click="emit('copyImage')" />
                <MenuRow :icon="icons.mdiContentCopy" label="Copy as JSON" @click="emit('copyText')" />
            </div>

            <div v-else class="menu__view">
                <MenuRow :icon="icons.mdiChevronLeft" label="Canvas" @click="show('main')" />
                <hr class="menu__rule" />
                <div class="menu__canvas">
                    <OriSelect v-model="sizeChoice" label="Canvas size" :options="sizeOptions" fluid />
                    <div v-if="sizeChoice === 'custom'" class="menu__size-custom">
                        <OriInput v-model="customW" label="W" type="number" min="1" max="8192" fluid />
                        <OriInput v-model="customH" label="H" type="number" min="1" max="8192" fluid />
                    </div>
                    <OriButton label="Apply size" variant="outline" radius="md" size="sm" @click="applySize" />
                    <OriSwitch
                        label="Checkerboard"
                        :model-value="props.backdropGrid"
                        @update:model-value="onToggleGrid"
                    />
                </div>
            </div>
        </OriSurface>
    </Teleport>
</template>

<style scoped>
/* A card hanging under the corner toggler; closed, it fades up and out. */
.menu {
    position: fixed;
    top: calc(var(--ori-size-gap_md, 0.5rem) * 2 + var(--ori-size-action_md, 2.75rem));
    right: var(--ori-size-gap_md, 0.5rem);
    z-index: 100;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_xs, 0.125rem);

    width: 20rem;
    max-height: calc(100dvh - 5rem);
    padding: var(--ori-size-gap_sm, 0.25rem);
    overflow-y: auto;

    color: var(--ori-color-on-surface);

    opacity: 0;
    transform: translateY(-0.5rem) scale(0.98);
    transform-origin: top right;
    visibility: hidden;
    transition:
        opacity 160ms ease-out,
        transform 200ms cubic-bezier(0.34, 1.4, 0.64, 1),
        visibility 0s linear 200ms;
}

.menu--open {
    opacity: 1;
    transform: none;
    visibility: visible;
    transition:
        opacity 160ms ease-out,
        transform 200ms cubic-bezier(0.34, 1.4, 0.64, 1);
}

.menu__name {
    margin: 0;
    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem) var(--ori-size-gap_xs, 0.125rem);

    overflow: hidden;

    font-weight: 700;
    white-space: nowrap;
    text-overflow: ellipsis;
}

.menu__name--unsaved {
    font-weight: 600;
    opacity: 0.7;
}

.menu__view {
    display: flex;
    flex-direction: column;
    gap: 1px;
}

.menu__rule {
    width: 100%;
    margin: var(--ori-size-gap_xs, 0.125rem) 0;

    border: none;
    border-top: 1px solid var(--jp-color-outline);
    opacity: 0.35;
}

.menu__theme {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--ori-size-gap_md, 0.5rem);

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_md, 0.5rem);
}

.menu__theme-icon {
    flex: none;
    opacity: 0.8;
}

.menu__canvas {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--ori-size-gap_md, 0.5rem);

    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);
}

.menu__size-custom {
    display: flex;
    gap: var(--ori-size-gap_md, 0.5rem);

    width: 100%;
}

.menu__profile {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.5rem);

    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);
}

.menu__who {
    display: flex;
    flex: 1;
    flex-direction: column;

    min-width: 0;
}

.menu__who-name {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
}

.menu__who-meta {
    font-size: var(--ori-font-size_sm, 0.875rem);
    opacity: 0.7;
}

.menu__rating {
    display: inline-flex;
    align-items: center;
    align-self: flex-start;

    color: inherit;
    text-decoration: none;
}

.menu__rating:hover {
    opacity: 1;
    text-decoration: underline;
}

/* The global focus ring covers buttons and inputs, not links. */
.menu__rating:focus-visible {
    outline: 2px solid var(--ori-color-primary);
    outline-offset: 2px;
}

/* A phone has no keyboard, and the panel becomes a right-edge drawer. */
@media (width <= 600px) {
    .menu__desktop-only {
        display: none;
    }

    .menu {
        top: 0;
        right: 0;

        width: min(20rem, calc(100vw - 3rem));
        height: 100dvh;
        max-height: none;
        padding-top: calc(var(--ori-size-gap_md, 0.5rem) * 2 + var(--ori-size-action_md, 2.75rem));

        transform: translateX(101%);
    }

    .menu--open {
        transform: none;
    }
}

@media (prefers-reduced-motion: reduce) {
    .menu,
    .menu--open {
        transition: none;
    }
}
</style>
