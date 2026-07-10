<script lang="ts">
import type { ToolId } from '@justpaint/editor'
import type { IconName } from './icons/ToolIcon.vue'

/**
 * Label + hotkey per tool — the SINGLE source of hotkey hint text. The toolbar
 * tooltips, the DrawView key bindings, and the shortcuts cheat-sheet all read
 * from here, so a remapped key can never drift out of sync with its hint.
 */
export const TOOL_META: Record<ToolId, { label: string; icon: IconName; key: string }> = {
    pen: { label: 'Pen', icon: 'pen', key: 'B' },
    eraser: { label: 'Eraser', icon: 'eraser', key: 'E' },
    line: { label: 'Line', icon: 'line', key: 'L' },
    rect: { label: 'Rectangle', icon: 'rect', key: 'R' },
    ellipse: { label: 'Ellipse', icon: 'ellipse', key: 'O' },
    triangle: { label: 'Triangle', icon: 'triangle', key: 'T' },
    hand: { label: 'Hand', icon: 'hand', key: 'H' }
}
</script>

<script lang="ts" setup>
/**
 * The floating bottom toolbar (tldraw-style — DECISIONS 2026-07-04): tools as
 * icon buttons, stroke/fill controls, undo/redo. Part of the shared /draw+/play
 * editor shell; file actions and panels live in the top clusters, not here.
 *
 * Structure is oriui-native (DESIGN-SYSTEM §4/§6): an `OriSurface` island holds
 * two `OriToolbar`s (WAI-ARIA roving-tabindex, one Tab stop each) — the 7 tools
 * as a single-select `OriToolbarToggleGroup`, and undo/redo as plain
 * `OriToolbarButton`s — with the stroke/fill form controls as a plain group
 * between them. Each item slots the app's multi-path `ToolIcon` through the
 * alpha-12 content slot (keeps our icon set) and carries `ori-button_icon` for
 * the square icon-mode sizing, identical to `IconButton`.
 *
 * The stroke/fill controls render TWICE with shared handlers: inline in the bar
 * (>600px) and inside a popover panel behind a swatch chip (<=600px, variant C).
 * A slot/component split is overkill for one consumer — CSS media queries show
 * exactly one copy per breakpoint.
 */
import {
    OriCheckbox,
    OriPopover,
    OriSlider,
    OriSurface,
    OriToolbar,
    OriToolbarButton,
    OriToolbarToggleGroup,
    OriToolbarToggleItem
} from '@oriui/vue'
import { TOOLS } from '@justpaint/editor'
import ToolIcon from './icons/ToolIcon.vue'

const toolIds = Object.keys(TOOLS) as ToolId[]

const props = defineProps<{
    activeTool: ToolId
    color: string
    strokeWidth: number
    fillEnabled: boolean
    fill: string
    canUndo: boolean
    canRedo: boolean
}>()

const emit = defineEmits<{
    pickTool: [id: ToolId]
    setColor: [hex: string]
    setWidth: [width: number]
    toggleFill: [enabled: boolean]
    setFill: [hex: string]
    undo: []
    redo: []
}>()

/**
 * `type="single"` is deselectable — clicking the active tool emits `undefined`
 * (Radix-style clear). A drawing tool must ALWAYS stay selected, so we drive the
 * group one-way off `props.activeTool` (the parent owns it) and ignore any clear
 * or non-tool value here; the group re-reads the unchanged model and keeps the
 * current tool pressed, no flicker.
 */
function onToolChange(value: string | string[] | undefined) {
    if (typeof value === 'string' && value in TOOLS) {
        emit('pickTool', value as ToolId)
    }
}

function onColor(e: Event) {
    emit('setColor', (e.target as HTMLInputElement).value)
}
function onFill(e: Event) {
    emit('setFill', (e.target as HTMLInputElement).value)
}
/** Free-typed px width: clamp to the slider's [1, 64] integer domain, reflect the clamp, emit. */
function onWidth(e: Event) {
    const el = e.target as HTMLInputElement
    const parsed = Math.round(Number(el.value))
    const width =
        el.value.trim() !== '' && Number.isFinite(parsed) ? Math.min(64, Math.max(1, parsed)) : props.strokeWidth
    // Write back so an out-of-range entry snaps visibly even when the emitted
    // value equals the current prop (no re-render to correct the field).
    el.value = String(width)
    emit('setWidth', width)
}
</script>

<template>
    <OriSurface as="div" class="bar" elevation="lg">
        <OriToolbar class="bar__toolbar" label="Drawing tools">
            <OriToolbarToggleGroup
                type="single"
                label="Tool"
                :model-value="props.activeTool"
                @update:model-value="onToolChange"
            >
                <span v-for="id in toolIds" :key="id" class="bar__tool-wrap">
                    <!-- Active tool = OriToolbar's own pressed affordance (a neutral 18% fill
                         + inset ring, from `[aria-pressed=true]`) plus a brand-tinted glyph
                         (`color="primary"`); resting tools are the neutral `surface` glyph.
                         This intended look was unblocked by oriui alpha-13, which fixed the two
                         bugs the alpha-12 migration surfaced (DESIGN-SYSTEM §6): the pressed
                         fill now paints `background-color` directly instead of the
                         layer-defeated `--ori-variant-bg-color` token, and `color` was dropped
                         from `.ori-button`'s transition so a per-selection glyph swap is instant
                         (no stuck relative-colour interpolation). -->
                    <OriToolbarToggleItem
                        class="ori-button_icon"
                        :value="id"
                        radius="md"
                        :color="props.activeTool === id ? 'primary' : 'surface'"
                        :tooltip="`${TOOL_META[id].label} — ${TOOL_META[id].key}`"
                    >
                        <ToolIcon :name="TOOL_META[id].icon" />
                    </OriToolbarToggleItem>
                    <!-- Excalidraw-style hotkey badge: discoverability hint, redundant with
                         the tooltip for AT (aria-hidden). Desktop-only — hidden <=600px.
                         The toggle item is icon-only, so the badge stays a sibling,
                         corner-positioned over it via .bar__tool-wrap. The wrapper span is
                         DOM-only — roving (a [data-ori-toolbar-item] DOM query) and the
                         group's provide/inject both see through it, so the a11y model is
                         untouched. -->
                    <span class="bar__tool-key" aria-hidden="true">{{ TOOL_META[id].key }}</span>
                </span>
            </OriToolbarToggleGroup>
        </OriToolbar>

        <span class="bar__divider" aria-hidden="true"></span>

        <!-- Inline style controls — visible >600px only. -->
        <div class="bar__group bar__style-inline" role="group" aria-label="Stroke and fill">
            <label class="bar__swatch" title="Stroke color">
                <input type="color" :value="props.color" aria-label="Stroke color" @input="onColor" />
            </label>

            <div class="bar__slider" :title="`Width — ${props.strokeWidth}`">
                <OriSlider
                    :model-value="props.strokeWidth"
                    :min="1"
                    :max="64"
                    :step="1"
                    aria-label="Stroke width"
                    @update:model-value="(v: number) => emit('setWidth', v)"
                />
                <input
                    class="bar__width-input"
                    type="number"
                    min="1"
                    max="64"
                    step="1"
                    :value="props.strokeWidth"
                    aria-label="Stroke width in px"
                    @change="onWidth"
                />
            </div>

            <div class="bar__fill">
                <OriCheckbox
                    :model-value="props.fillEnabled"
                    label="Fill"
                    @update:model-value="(v) => emit('toggleFill', v === true)"
                />
                <label class="bar__swatch" title="Fill color">
                    <input
                        type="color"
                        :value="props.fill"
                        :disabled="!props.fillEnabled"
                        aria-label="Fill color"
                        @input="onFill"
                    />
                </label>
            </div>
        </div>

        <!--
            Mobile style popover (variant C) — visible <=600px only. Toggle, light
            dismiss, and Esc come from the native HTML Popover API; positioning is
            CSS anchor (placement="top" — the bar sits bottom-center). NB: CSS
            anchor positioning isn't in Firefox yet, so there the panel opens
            viewport-centered (the [popover] UA default) — acceptable.
        -->
        <OriPopover placement="top">
            <template #trigger="{ props: popoverTrigger }">
                <!-- Cast: the slot types aria-haspopup as plain string; Vue's
                     ButtonHTMLAttributes wants its literal union. -->
                <button
                    v-bind="popoverTrigger as Record<string, unknown>"
                    class="bar__tool bar__style-trigger"
                    type="button"
                    aria-label="Stroke & fill"
                >
                    <span class="bar__style-dot" :style="{ background: props.color }" aria-hidden="true"></span>
                </button>
            </template>

            <div class="bar__style-panel">
                <label class="bar__swatch" title="Stroke color">
                    <input type="color" :value="props.color" aria-label="Stroke color" @input="onColor" />
                </label>

                <div class="bar__slider" :title="`Width — ${props.strokeWidth}`">
                    <OriSlider
                        :model-value="props.strokeWidth"
                        :min="1"
                        :max="64"
                        :step="1"
                        aria-label="Stroke width"
                        @update:model-value="(v: number) => emit('setWidth', v)"
                    />
                    <input
                        class="bar__width-input"
                        type="number"
                        min="1"
                        max="64"
                        step="1"
                        :value="props.strokeWidth"
                        aria-label="Stroke width in px"
                        @change="onWidth"
                    />
                </div>

                <div class="bar__fill">
                    <OriCheckbox
                        :model-value="props.fillEnabled"
                        label="Fill"
                        @update:model-value="(v) => emit('toggleFill', v === true)"
                    />
                    <label class="bar__swatch" title="Fill color">
                        <input
                            type="color"
                            :value="props.fill"
                            :disabled="!props.fillEnabled"
                            aria-label="Fill color"
                            @input="onFill"
                        />
                    </label>
                </div>
            </div>
        </OriPopover>

        <span class="bar__divider bar__divider--history" aria-hidden="true"></span>

        <OriToolbar class="bar__toolbar bar__toolbar--history" label="History">
            <OriToolbarButton
                class="ori-button_icon"
                radius="md"
                color="surface"
                tooltip="Undo — Ctrl/⌘+Z"
                :disabled="!props.canUndo"
                @click="emit('undo')"
            >
                <ToolIcon name="undo" />
            </OriToolbarButton>
            <OriToolbarButton
                class="ori-button_icon"
                radius="md"
                color="surface"
                tooltip="Redo — Ctrl/⌘+Y"
                :disabled="!props.canRedo"
                @click="emit('redo')"
            >
                <ToolIcon name="redo" />
            </OriToolbarButton>
        </OriToolbar>
    </OriSurface>
</template>

<style scoped>
.bar {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.5rem);

    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);

    /* Never wider than the viewport; inner groups scroll-free compact on phones. */
    max-width: calc(100vw - 1rem);
}

.bar__group {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.bar__divider {
    align-self: stretch;
    width: 1px;
    margin: 0.2rem 0.15rem;
    background-color: var(--ori-color-outline, rgb(0 0 0 / 12%));
}

/* Base chrome for the one remaining raw button — the mobile stroke/fill popover
   trigger (.bar__style-trigger below), a bespoke swatch-preview control the
   toolbar items can't express (their content is a fixed ToolIcon glyph, not a
   live color dot). The tools and undo/redo now render via OriToolbar* items,
   which own their own box model, so this rule (and its hover) no longer reaches
   any converted button. */
.bar__tool {
    position: relative;

    display: grid;
    place-items: center;

    width: var(--jp-control-lg, 2.4rem);
    height: var(--jp-control-lg, 2.4rem);
    padding: 0;

    border: none;
    border-radius: var(--ori-size-radius_md, 8px);
    background: transparent;
    color: var(--ori-color-on-surface);

    font-size: 1rem;
    cursor: pointer;
    transition:
        background-color 120ms ease,
        color 120ms ease;
}

.bar__tool:hover:not(:disabled) {
    /* Neutral overlay off a structural token — NOT a brand role (DESIGN-SYSTEM
       §1). The only control this still styles is the bespoke .bar__style-trigger
       swatch, which the toolbar can't express (a live colour dot, not a glyph). */
    background-color: var(--jp-neutral-hover-bg, color-mix(in srgb, var(--ori-color-on-surface) 8%, transparent));
}

/* Positioning context for the hotkey badge below. The toggle item is icon-only
   (no slot for a corner badge), so the badge renders as a sibling; this wrapper
   shrink-wraps to the button so the badge's corner offset lands on the button's
   own edge, matching the pre-migration layout where the badge sat over it. */
.bar__tool-wrap {
    position: relative;
    display: inline-flex;
}

/* Excalidraw-style hotkey badge — corner glyph on the 7 tool buttons only.
   Absolute so it never nudges the centered icon out of place. */
.bar__tool-key {
    position: absolute;
    right: 0.2rem;
    bottom: 0.1rem;

    color: color-mix(in srgb, var(--ori-color-on-surface) 55%, transparent);

    font-size: 9px;
    font-weight: 700;
    line-height: 1;

    pointer-events: none;
}

.bar__swatch {
    position: relative;
    display: grid;
    place-items: center;
}

.bar__swatch input[type='color'] {
    width: var(--jp-control-lg, 2.4rem);
    height: var(--jp-control-lg, 2.4rem);
    padding: 0;

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 20%));
    border-radius: 50%;
    background: none;

    cursor: pointer;
}

/* Native color wells insist on their own swatch padding — trim it to the ring. */
.bar__swatch input[type='color']::-webkit-color-swatch-wrapper {
    padding: 2px;
}

.bar__swatch input[type='color']::-webkit-color-swatch {
    border: none;
    border-radius: 50%;
}

.bar__swatch input[type='color']:disabled {
    opacity: 0.35;
    cursor: default;
}

.bar__fill {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.bar__slider {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
    width: 10rem;
}

.bar__width-input {
    flex: none;
    width: 3.2rem;
    padding: 0.15rem 0.3rem;

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 20%));
    border-radius: var(--ori-size-radius_sm, 4px);
    background: transparent;
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_xs, 0.75rem);
    font-variant-numeric: tabular-nums;
    text-align: right;

    /* Spinners would eat most of 3.2rem — hide them; the slider is the coarse control. */
    appearance: textfield;
}

.bar__width-input::-webkit-outer-spin-button,
.bar__width-input::-webkit-inner-spin-button {
    margin: 0;
    appearance: none;
}

/* The popover trigger chip: current stroke color as a ringed dot. */
.bar__style-dot {
    width: 1.25rem;
    height: 1.25rem;

    border: 2px solid var(--ori-color-surface, #ffffff);
    border-radius: 50%;
    box-shadow: 0 0 0 1px var(--ori-color-outline, rgb(0 0 0 / 20%));
}

/* The popover panel: vertical stack of the same stroke/fill controls. */
.bar__style-panel {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: var(--ori-size-gap_md, 0.5rem);

    min-width: 14rem;
    padding: var(--ori-size-gap_md, 0.5rem);
}

.bar__style-panel .bar__slider {
    width: auto;
}

.bar__style-panel .bar__swatch {
    justify-items: start;
}

/* Desktop: the style controls live inline; the popover chip (and its panel) hide. */
@media (width > 600px) {
    .bar__style-trigger,
    .bar__style-panel {
        display: none;
    }
}

/* Phones: the style group collapses into the popover chip so the bar fits one
   row at 360-430px; flex-wrap stays as the safety net. */
@media (width <= 600px) {
    .bar {
        flex-wrap: wrap;
        justify-content: center;
        gap: var(--ori-size-gap_sm, 0.25rem);
        padding: 0.35rem 0.5rem;
    }

    .bar__style-inline {
        display: none;
    }

    /* Compact chrome: dividers off and tighter buttons — 6 tools + chip fit
       one row inside a 360px viewport minus margins. (.bar__divider covers the
       history divider too — it carries both classes.) */
    .bar__divider {
        display: none;
    }

    /* Undo/redo move to the host view's top-left history island on phones
       (accidental-tap protection next to the tools). */
    .bar__toolbar--history {
        display: none;
    }

    /* Shrink the icon-mode tool squares by repointing the size TOKEN (the §0
       escape-hatch, not a state/colour override) on the button itself — it must
       land ON .ori-button_icon, since .ori-button_md re-declares
       --ori-size-action there and would shadow an inherited value. This unlayered
       rule (0,3,0 incl. the scope attr) outranks oriui's layered token, and lands
       only on the 7 tool buttons (undo/redo sit outside .bar__tool-wrap). 32px
       keeps 7 tools + the style chip compact at 360px. */
    .bar__tool-wrap :deep(.ori-button) {
        --ori-size-action: 2rem;
    }

    .bar__tool {
        width: 2rem;
        height: 2rem;
    }

    /* Touch phones have no hardware keyboard — drop the hotkey badges (matches
       how the bar hides its other keyboard affordances at this breakpoint). */
    .bar__tool-key {
        display: none;
    }

    /* Color wells now only render inside the popover panel — keep them tappable. */
    .bar__swatch input[type='color'] {
        width: var(--jp-control-sm, 2.25rem);
        height: var(--jp-control-sm, 2.25rem);
    }
}
</style>
