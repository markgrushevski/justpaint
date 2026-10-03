<script lang="ts" setup>
/**
 * The /draw menu: the file actions, the canvas colour, the look of the app and the account.
 * On a wide screen it is a non-modal panel under the corner toggler, so the canvas stays
 * live; on a phone it is a modal drawer from the right edge. Export and canvas size open as
 * sub-panels in place. Esc steps back from a sub-panel, then closes.
 */
import { computed, nextTick, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import {
    OriAvatar,
    OriButton,
    OriDivider,
    OriDrawer,
    OriInput,
    OriList,
    OriListItem,
    OriSegmentedControl,
    OriSelect,
    OriSwitch
} from '@oriui/vue'
import { icons, useAuthGate, useMediaQuery, useSessionStore, useThemeStore } from '@core'
import type { Accent, ThemeMode } from '@core'
import IslandSurface from '../../components/ui/IslandSurface.vue'
import SwatchPicker from '../../components/ui/SwatchPicker.vue'
import type { SwatchOption } from '../../components/ui/SwatchPicker.vue'
import ToolIcon from '../../components/icons/ToolIcon.vue'
import type { IconName } from '../../components/icons/ToolIcon.vue'
import { CANVAS_COLORS } from './canvasColors'

const props = defineProps<{
    open: boolean
    busy: boolean
    /** The drawing's name once saved, or null while it has never been saved. */
    title: string | null
    backdropGrid: boolean
    canvasWidth: number
    canvasHeight: number
    /** The drawing's canvas colour, or null for plain paper. */
    background: string | null
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
    setBackground: [color: string | null]
}>()

const session = useSessionStore()
const theme = useThemeStore()
const gate = useAuthGate()

// A phone has no room beside the canvas and no keyboard for the shortcuts.
const phone = useMediaQuery('(width <= 600px)')

type View = 'main' | 'export' | 'size'
const view = ref<View>('main')

// Icons only on screen; the names stay for assistive technology.
const THEME_OPTIONS: { value: ThemeMode; label: string }[] = [
    { value: 'light', label: 'Light' },
    { value: 'dark', label: 'Dark' },
    { value: 'auto', label: 'Auto' }
]
const THEME_ICONS: Record<ThemeMode, IconName> = { light: 'sun', dark: 'moon', auto: 'auto' }

function themeIcon(value: string | number): IconName {
    return THEME_ICONS[THEME_OPTIONS.find((o) => o.value === value)?.value ?? 'auto']
}

function selectTheme(mode: string | number | undefined): void {
    const next = THEME_OPTIONS.find((o) => o.value === mode)
    if (next) theme.mode = next.value
}

// The dots show each accent as the current theme paints it; the values are main.css's.
type PresetAccent = Exclude<Accent, 'custom'>
const ACCENT_SWATCHES: Record<PresetAccent, { label: string; light: string; dark: string }> = {
    orange: { label: 'Orange', light: 'hsl(20 100% 50%)', dark: 'hsl(20 100% 60%)' },
    green: { label: 'Green', light: 'hsl(125 100% 20%)', dark: 'hsl(125 60% 50%)' },
    blue: { label: 'Blue', light: 'hsl(222 90% 50%)', dark: 'hsl(222 90% 68%)' },
    violet: { label: 'Violet', light: 'hsl(268 80% 52%)', dark: 'hsl(268 85% 75%)' }
}
const PRESET_ACCENTS = Object.keys(ACCENT_SWATCHES) as PresetAccent[]
const accentOptions = computed<SwatchOption[]>(() =>
    PRESET_ACCENTS.map((value) => {
        const swatch = ACCENT_SWATCHES[value]
        return {
            value,
            label: swatch.label,
            color: theme.isDark ? swatch.dark : swatch.light,
            ink: 'var(--ori-color-on-primary)'
        }
    })
)
const accentCustom = computed(() => ({
    label: 'Custom accent',
    color: theme.customAccent,
    active: theme.accent === 'custom'
}))

function selectAccent(value: string | null): void {
    const next = PRESET_ACCENTS.find((a) => a === value)
    if (next) theme.setAccent(next)
}

// A colour that isn't one of the presets is the custom one.
const canvasCustom = computed(() => {
    const color = props.background
    const active = color !== null && !CANVAS_COLORS.some((c) => c.value === color)
    return { label: 'Custom canvas colour', color: active && color ? color : '#ffffff', active }
})

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

const body = ref<HTMLElement | null>(null)

/** Focus the first control of the panel now showing, so arrow-free keyboard use continues there. */
async function focusFirst() {
    await nextTick()
    body.value?.querySelector<HTMLElement>('.menu__view button, .menu__view a, .menu__view input')?.focus()
}

function show(next: View) {
    view.value = next
    focusFirst()
}

// Opening resets the panel and moves focus inside, or Esc (keydown on the panel tree)
// never fires; closing returns focus to whatever opened it (the drawer does that itself too).
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

// The view's own Esc handler on window would close the whole panel too. While a colour
// picker's popover is open, Esc closes only that, which the popover does itself. In a
// sub-panel Esc steps back, and preventing it keeps the drawer from closing as well.
function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return
    e.stopPropagation()
    if (body.value?.querySelector('[popover]:popover-open')) return
    if (view.value !== 'main') {
        e.preventDefault()
        show('main')
    } else if (!phone.value) emit('close')
}

function onDrawer(open: boolean) {
    if (!open) emit('close')
}

const name = computed(() => props.title ?? 'Unsaved drawing')

// The two shells take different props, so the one in use gets its own set.
const shell = computed(() =>
    phone.value
        ? { open: props.open, side: 'end', 'onUpdate:open': onDrawer }
        : {
              as: 'aside',
              class: ['menu', { 'menu--open': props.open }],
              elevation: 'lg',
              role: 'complementary',
              'aria-label': 'Menu',
              tabindex: -1,
              inert: !props.open
          }
)
</script>

<template>
    <Teleport to="body">
        <component :is="phone ? OriDrawer : IslandSurface" v-bind="shell">
            <template v-if="phone" #title>
                <span :class="{ 'menu__name--unsaved': !props.title }">{{ name }}</span>
            </template>
            <p v-if="!phone" class="menu__name" :class="{ 'menu__name--unsaved': !props.title }">{{ name }}</p>

            <div ref="body" class="menu__body" :class="{ 'menu__body--drawer': phone }" @keydown="onKeydown">
                <div v-if="view === 'main'" class="menu__view">
                    <OriList>
                        <OriListItem :icon="icons.mdiPlus" label="New drawing" @click="run(() => emit('newDrawing'))" />
                        <OriListItem
                            :as="RouterLink"
                            to="/gallery"
                            :icon="icons.mdiImageMultipleOutline"
                            label="My drawings"
                        />
                        <OriListItem
                            :icon="icons.mdiContentSaveOutline"
                            label="Save"
                            hint="Ctrl+S"
                            :disabled="props.busy"
                            @click="run(() => emit('save'))"
                        />
                        <OriListItem :icon="icons.mdiDownload" label="Export" chevron @click="show('export')" />
                    </OriList>

                    <OriDivider />

                    <div class="menu__settings">
                        <div class="menu__setting">
                            <span class="menu__setting-label" aria-hidden="true">Canvas</span>
                            <SwatchPicker
                                :model-value="props.background"
                                :options="CANVAS_COLORS"
                                label="Canvas colour"
                                ink-view
                                :custom="canvasCustom"
                                @update:model-value="(color) => emit('setBackground', color)"
                                @custom="(color) => emit('setBackground', color)"
                            />
                        </div>
                        <OriSwitch
                            label="Invert in the dark theme"
                            :model-value="theme.invertCanvas"
                            @update:model-value="(on) => theme.setInvertCanvas(on === true)"
                        />
                    </div>
                    <OriList>
                        <OriListItem
                            :icon="icons.mdiAspectRatio"
                            label="Canvas size"
                            :hint="`${props.canvasWidth} × ${props.canvasHeight}`"
                            chevron
                            @click="show('size')"
                        />
                    </OriList>

                    <OriDivider />

                    <div class="menu__settings">
                        <div class="menu__setting">
                            <span class="menu__setting-label" aria-hidden="true">Theme</span>
                            <OriSegmentedControl
                                :model-value="theme.mode"
                                :options="THEME_OPTIONS"
                                aria-label="Theme"
                                size="sm"
                                @update:model-value="selectTheme"
                            >
                                <template #option="{ option }">
                                    <ToolIcon :name="themeIcon(option.value)" />
                                    <span class="jp-sr-only">{{ option.label }}</span>
                                </template>
                            </OriSegmentedControl>
                        </div>
                        <div class="menu__setting">
                            <span class="menu__setting-label" aria-hidden="true">Accent</span>
                            <SwatchPicker
                                :model-value="theme.accent === 'custom' ? null : theme.accent"
                                :options="accentOptions"
                                label="Accent colour"
                                :custom="accentCustom"
                                @update:model-value="selectAccent"
                                @custom="(color) => theme.setAccent('custom', color)"
                            />
                        </div>
                    </div>

                    <OriDivider v-if="!phone" />

                    <OriList v-if="!phone">
                        <OriListItem
                            :icon="icons.mdiKeyboard"
                            label="Keyboard shortcuts"
                            hint="?"
                            @click="run(() => emit('shortcuts'))"
                        />
                    </OriList>

                    <OriDivider />

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
                            </RouterLink>
                        </div>
                        <OriButton label="Log out" variant="outline" radius="md" size="sm" @click="logout" />
                    </div>
                    <OriList v-else>
                        <OriListItem :icon="icons.mdiLogin" label="Sign in" @click="signIn" />
                    </OriList>
                </div>

                <div v-else-if="view === 'export'" class="menu__view">
                    <OriList>
                        <OriListItem :icon="icons.mdiChevronLeft" label="Export" @click="show('main')" />
                    </OriList>
                    <OriDivider />
                    <OriList>
                        <OriListItem
                            :icon="icons.mdiDownload"
                            label="Download PNG"
                            @click="run(() => emit('exportPng'))"
                        />
                        <!-- Copying leaves the panel open: the next paste is somewhere else. -->
                        <OriListItem :icon="icons.mdiContentCopy" label="Copy as image" @click="emit('copyImage')" />
                        <OriListItem :icon="icons.mdiContentCopy" label="Copy as JSON" @click="emit('copyText')" />
                    </OriList>
                </div>

                <div v-else class="menu__view">
                    <OriList>
                        <OriListItem :icon="icons.mdiChevronLeft" label="Canvas size" @click="show('main')" />
                    </OriList>
                    <OriDivider />
                    <div class="menu__size">
                        <OriSelect v-model="sizeChoice" label="Size" :options="sizeOptions" fluid />
                        <div v-if="sizeChoice === 'custom'" class="menu__size-custom">
                            <OriInput v-model="customW" label="W" type="number" min="1" max="8192" fluid />
                            <OriInput v-model="customH" label="H" type="number" min="1" max="8192" fluid />
                        </div>
                        <OriButton label="Apply size" variant="outline" radius="md" size="sm" @click="applySize" />
                        <!-- Plain paper exports transparent; the checkerboard shows where. -->
                        <OriSwitch
                            label="Checkerboard on plain paper"
                            :model-value="props.backdropGrid"
                            @update:model-value="onToggleGrid"
                        />
                    </div>
                </div>
            </div>
        </component>
    </Teleport>
</template>

<style scoped>
/* A card hanging one gap under the corner toggler; closed, it fades up and out. */
.menu {
    position: fixed;
    top: calc(
        var(--ori-size-gap_md, 0.5rem) * 2 + var(--ori-size-action_md, 2.75rem) + var(--ori-size-gap_xs, 0.125rem) * 2
    );
    right: var(--ori-size-gap_md, 0.5rem);
    z-index: 100;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_xs, 0.125rem);

    width: 20rem;
    max-height: calc(100dvh - 5rem);
    padding: var(--ori-size-gap_md, 0.5rem);
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
    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_lg, 0.75rem) var(--ori-size-gap_sm, 0.25rem);

    overflow: hidden;

    font-weight: 700;
    white-space: nowrap;
    text-overflow: ellipsis;
}

.menu__name--unsaved {
    font-weight: 600;
    opacity: var(--jp-dim, 0.7);
}

.menu__body {
    display: flex;
    flex-direction: column;
}

/* The drawer pads its body and sets a smaller type size; the rows bring their own inset, so
   they reach out to line their icons up with the drawer's title, at the panel's type size. */
.menu__body--drawer {
    margin-inline: calc(var(--ori-size-gap_lg, 0.75rem) * -1);

    font-size: var(--ori-font-size_md, 1rem);
}

.menu__view {
    display: flex;
    flex-direction: column;
}

/* Settings sit on the list rows' inset, so their labels line up with the row icons. */
.menu__settings {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_lg, 0.75rem);

    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_lg, 0.75rem);
}

.menu__setting {
    display: grid;
    grid-template-columns: 4.25rem 1fr;
    align-items: start;
    gap: var(--ori-size-gap_md, 0.5rem);
}

/* Centred on the first line of dots or segments. */
.menu__setting-label {
    display: flex;
    align-items: center;

    min-height: var(--ori-size-action_sm, 1.5rem);

    font-size: var(--ori-font-size_sm, 0.875rem);
    font-weight: 700;
}

.menu__size {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--ori-size-gap_lg, 0.75rem);

    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_lg, 0.75rem) var(--ori-size-gap_md, 0.5rem);
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

    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_lg, 0.75rem);
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
    opacity: var(--jp-dim, 0.7);
}

/* A link looks like one: underlined, no arrow. */
.menu__rating {
    align-self: flex-start;

    color: inherit;
    text-decoration: underline;
    text-underline-offset: 2px;
}

.menu__rating:hover {
    opacity: 1;
}

/* The global focus ring covers buttons and inputs, not links. */
.menu__rating:focus-visible {
    outline: 2px solid var(--ori-color-primary);
    outline-offset: 2px;
}

@media (prefers-reduced-motion: reduce) {
    .menu,
    .menu--open {
        transition: none;
    }
}
</style>
