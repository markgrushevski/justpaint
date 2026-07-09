<script lang="ts" setup>
/**
 * EditorShell — the shared editor LAYOUT SKELETON for /draw and (later) /play
 * (DECISIONS 2026-07-04: one design, game chrome on top). It owns ONLY the
 * full-bleed desk/letterbox surface, the Konva canvas MOUNT element, and the
 * absolutely-positioned floating regions; every piece of chrome is a caller
 * slot. The parent constructs the Editor into the exposed `canvasEl` (in its own
 * onMounted) and fills the regions with its own (draw or play) chrome.
 *
 * Regions: `#top-left #top-center #top-right #bottom-left #bottom-center
 * #bottom-right` are positioned wrappers; `#overlay` is the centered
 * dialog/empty-state layer; `#drawer` and the default slot render as direct
 * children of the root for self-positioned / body-teleported chrome.
 *
 * The `*-center` strips + `#overlay` set `pointer-events: none` so their empty
 * area never intercepts canvas drawing — slotted content that must be
 * interactive opts back in with `pointer-events: auto`. The `#bottom-left` is a
 * passive-readout corner (also `pointer-events: none`).
 *
 * Stacking: the root is `position: relative` with NO z-index/transform, so it is
 * NOT a stacking context — the region z-indexes and any body-teleported overlays
 * (the side drawer, the dialogs) all compare in the ONE root stacking context,
 * exactly as the old `.draw` wrapper did. Do NOT add z-index/transform/opacity/
 * filter/isolation to `.shell` or a corner-pinned control (e.g. /draw's z-110
 * menu toggler) would fall behind the z-100 teleported drawer.
 */
import { ref } from 'vue'

withDefaults(defineProps<{ mode?: 'draw' | 'play' }>(), { mode: 'draw' })

// The Konva mount element, exposed so the parent can `new Editor(canvasEl, …)`
// in its own onMounted (the editor sizes its stage to this box and fits the
// document into it; a ResizeObserver keeps it fitted). Exposed as the ref — the
// parent reads `shell.value.canvasEl` (Vue's expose proxy unwraps it to the
// element).
const canvasEl = ref<HTMLDivElement | null>(null)
defineExpose({ canvasEl })
</script>

<template>
    <div class="shell" :class="`shell--${mode}`" :data-mode="mode">
        <!-- The Editor sizes its Konva stage to this full-bleed mount and fits the
             document into it (zoom/pan via the stage transform). Everything else
             floats above it through the region slots. -->
        <div ref="canvasEl" class="shell__canvas"></div>

        <div v-if="$slots['top-left']" class="shell__region shell__region--top-left">
            <slot name="top-left" />
        </div>
        <div v-if="$slots['top-center']" class="shell__region shell__region--top-center">
            <slot name="top-center" />
        </div>
        <div v-if="$slots['top-right']" class="shell__region shell__region--top-right">
            <slot name="top-right" />
        </div>
        <div v-if="$slots['bottom-left']" class="shell__region shell__region--bottom-left">
            <slot name="bottom-left" />
        </div>
        <div v-if="$slots['bottom-center']" class="shell__region shell__region--bottom-center">
            <slot name="bottom-center" />
        </div>
        <div v-if="$slots['bottom-right']" class="shell__region shell__region--bottom-right">
            <slot name="bottom-right" />
        </div>

        <!-- The side drawer (self-teleports/positions) rendered BEFORE the overlay
             so a body-teleported drawer stays behind the equally-ranked (z-100)
             body-teleported dialogs, exactly as the old markup ordered them. -->
        <slot name="drawer" />

        <!-- Centered overlay layer (empty-state / dialogs). pointer-events:none so
             it never blocks drawing; interactive slotted content opts back in.
             z-11 keeps it above the bottom-center toolbar (z-10). -->
        <div v-if="$slots.overlay" class="shell__overlay">
            <slot name="overlay" />
        </div>

        <!-- Free-floating, self-positioned chrome the caller owns (e.g. /draw's
             corner menu toggler, mobile history island, layers panel + scrim).
             Direct children of the non-stacking-context root, so their own
             z-index / absolute positioning resolve against the shell as before. -->
        <slot />
    </div>
</template>

<style scoped>
.shell {
    position: relative;
    height: 100%;
    overflow: hidden;

    /* Letterbox "desk" around the fitted document (the editor paints the paper +
       shadow on top). One step off the paper so the document edge reads at any
       zoom — never the paper's own color. */
    background-color: var(--jp-desk, #e9ebef);
}

.shell__canvas {
    position: absolute;
    inset: 0;
}

/* --- floating regions (positioned wrappers; the caller fills each slot) ------ */

.shell__region {
    position: absolute;
    z-index: 10;
}

.shell__region--top-left,
.shell__region--top-right {
    top: var(--ori-size-gap_md, 0.5rem);

    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.shell__region--top-left {
    left: var(--ori-size-gap_md, 0.5rem);
}

/* Shifted left of a corner toggler slot (gap + action + gap). Max width =
   viewport minus that offset minus a left breathing gap; on very narrow phones
   the chips wrap to a second row inside the island. */
.shell__region--top-right {
    right: calc(var(--ori-size-gap_md, 0.5rem) * 2 + var(--ori-size-action_md, 2.75rem));
    max-width: calc(100vw - (var(--ori-size-gap_md, 0.5rem) * 3 + var(--ori-size-action_md, 2.75rem)));
}

/* /play has no corner menu toggler, so its top-right island reclaims the full
   corner — Submit sits at the edge, clear of the centered round-timer clock on
   narrow phones. (The one place the `mode` prop tunes the shared layout.) */
.shell--play .shell__region--top-right {
    right: var(--ori-size-gap_md, 0.5rem);
    max-width: calc(100vw - var(--ori-size-gap_md, 0.5rem) * 2);
}

/* Full-width centering strip (NOT left:50% + translate: an offset absolute box
   shrink-to-fits against the REMAINING half of the viewport and wraps on
   phones). The strip must not eat canvas events — content opts back in. */
.shell__region--top-center,
.shell__region--bottom-center {
    left: 0;
    right: 0;

    display: flex;
    justify-content: center;

    pointer-events: none;
}

.shell__region--top-center {
    top: var(--ori-size-gap_md, 0.5rem);
}

.shell__region--bottom-center {
    bottom: var(--ori-size-gap_lg, 0.75rem);
}

.shell__region--bottom-right {
    right: var(--ori-size-gap_md, 0.5rem);
    bottom: var(--ori-size-gap_lg, 0.75rem);
}

/* Passive-readout corner (e.g. /draw's coords): pointer-events:none so a chip
   here never intercepts canvas drawing. */
.shell__region--bottom-left {
    left: var(--ori-size-gap_md, 0.5rem);
    bottom: var(--ori-size-gap_lg, 0.75rem);

    pointer-events: none;
}

/* Centered layer over the canvas (empty-state card, and a home for the
   body-teleported dialogs/toaster). pointer-events:none so it never blocks
   drawing; only opted-in slotted content is interactive. z-11 > toolbar z-10. */
.shell__overlay {
    position: absolute;
    inset: 0;
    z-index: 11;

    display: grid;
    place-items: center;

    pointer-events: none;
}

/* --- small screens ----------------------------------------------------------- */

@media (width <= 600px) {
    .shell__region--bottom-center {
        bottom: var(--ori-size-gap_sm, 0.25rem);
    }

    /* Zoom tucks into the bottom-right above the one-row toolbar; the top corners
       stay free for the history island (left) and the actions row (right). */
    .shell__region--bottom-right {
        right: var(--ori-size-gap_sm, 0.25rem);
        bottom: 4.25rem;
    }
}
</style>
