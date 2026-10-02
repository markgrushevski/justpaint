<script lang="ts" setup>
/**
 * EditorShell — the shared editor layout skeleton for /draw, /play and
 * /practice: it owns only the full-bleed desk/letterbox surface, the Konva
 * canvas mount, and the absolutely-positioned floating regions (`#top-left
 * #top-center #top-right #bottom-left #bottom-center #bottom-right`,
 * `#overlay`, `#drawer`), leaving every piece of chrome to a caller slot. The
 * parent constructs the Editor into the exposed `canvasEl` in its own
 * `onMounted`. The `*-center` strips and `#overlay` are `pointer-events: none`
 * so their empty area never intercepts canvas drawing; interactive slotted
 * content opts back in with `pointer-events: auto`.
 *
 * Stacking: the root is `position: relative` with no z-index/transform, so it
 * is not a stacking context — region z-indexes and body-teleported overlays
 * (the drawer, the dialogs) all compare in the one root stacking context. Do
 * not add z-index/transform/opacity/filter/isolation to `.shell`, or a
 * corner-pinned control (e.g. /draw's z-110 menu toggler) would fall behind
 * the z-100 teleported drawer.
 */
import { ref } from 'vue'

withDefaults(defineProps<{ mode?: 'draw' | 'play' }>(), { mode: 'draw' })

// The Konva mount element, exposed so the parent can `new Editor(canvasEl, …)`
// in its own onMounted — the editor sizes its stage to this box and a
// ResizeObserver keeps it fitted. The parent reads `shell.value.canvasEl`
// (Vue's expose proxy unwraps the ref to the element).
const canvasEl = ref<HTMLDivElement | null>(null)
defineExpose({ canvasEl })
</script>

<template>
    <div class="shell" :class="`shell--${mode}`" :data-mode="mode">
        <!-- A free drawing goes through the dark theme's ink view (main.css); a scored one
             stays the judge's white sheet in both themes. -->
        <div ref="canvasEl" class="shell__canvas" :class="{ 'jp-ink-view': mode === 'draw' }"></div>

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

        <!-- Rendered before the overlay so the body-teleported drawer stays
             behind the equally-ranked (z-100) body-teleported dialogs. -->
        <slot name="drawer" />

        <!-- pointer-events:none so it never blocks drawing; interactive slotted
             content opts back in. z-11 keeps it above the bottom-center toolbar
             (z-10). -->
        <div v-if="$slots.overlay" class="shell__overlay">
            <slot name="overlay" />
        </div>

        <!-- Free-floating, self-positioned chrome the caller owns (e.g. /draw's
             corner menu toggler, mobile history island, layers panel + scrim) —
             direct children of the non-stacking-context root. -->
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

/* Full-width centering strip, not left:50% + translate — an offset absolute
   box shrink-to-fits against the remaining half of the viewport and wraps on
   phones. Must not eat canvas events — content opts back in. */
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

/* Centered layer over the canvas (the /draw welcome, result cards, and a home for the
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

/* A <dialog> renders in the browser's top layer, but `pointer-events` still
   inherits down the DOM tree — so a modal parked in this layer looked normal and
   swallowed every click: its × and its backdrop were dead, while Esc still
   worked because that is the keyboard, not the pointer. Any dialog under this
   overlay opts back in, once, here — through `:deep()`, because the <dialog>
   belongs to a child component and never carries this component's scope id. */
.shell__overlay :deep(dialog) {
    pointer-events: auto;
}

/* The zoom island and the toolbar share the bottom row, and the island has to
   move ABOVE the toolbar for every width where the toolbar is wide enough to
   reach it — which is far more than "phones".

   The toolbar is centered and shrink-to-fits; its intrinsic width on /draw
   measures 769px. The island is 192px wide, anchored `--ori-size-gap_md` (8px)
   from the right edge. They stop touching when the toolbar's right edge clears
   the island's left edge:

       (vw + 769) / 2  <=  vw - (192 + 8)      =>   vw >= 1169

   Keying the lift on the phone breakpoint instead left the whole 601-1169px
   band overlapping — every tablet, and every phone held sideways (measured
   9580px^2 at 768x1024 and at 667x375, 3605px^2 at 1024x768). 1200
   rather than 1169 leaves the toolbar ~31px of room to grow before the number
   is wrong again; `tests/layout/chrome-overlap.spec.ts` is what notices if it
   ever does, because nothing else in this repo looks at geometry. */
@media (width <= 1200px) {
    /* Clears the toolbar's own bottom offset plus its height (54px at full
       size, 45px in the compact phone form) with enough left over that the two
       read as separate islands rather than one stack. */
    .shell__region--bottom-right {
        bottom: 5rem;
    }
}

@media (width <= 600px) {
    .shell__region--bottom-center {
        bottom: var(--ori-size-gap_sm, 0.25rem);
    }

    /* Phone gutters tighten so the top corners keep their space for the history
       island (left) and the actions row (right). */
    .shell__region--bottom-right {
        right: var(--ori-size-gap_sm, 0.25rem);
    }
}
</style>
