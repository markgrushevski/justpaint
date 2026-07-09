<script lang="ts" setup>
/**
 * JpFloat — the elevated floating-surface island for canvas chrome (toolbars,
 * zoom, panels, corner actions) over the full-bleed editor. oriui has no
 * elevation primitive — OriCard is a flat content card with header/title
 * semantics and `gap_xl` padding, wrong for chrome — so this is the one
 * justpaint UI primitive for "a lifted surface over the canvas"
 * (docs/DESIGN-SYSTEM.md §4). Thin by design: it owns only the shared surface
 * look (the `.jp-float` token in main.css — one definition, surface + hairline +
 * radius_lg + drop shadow); callers add their own layout. `as` swaps the element
 * for the right semantics (e.g. a passive readout stays a `div`, a labelled
 * group can be a `section`). Upstream candidate: an oriui `OriSheet`/elevation
 * primitive would let this become a re-export (DESIGN-SYSTEM §6).
 */
withDefaults(defineProps<{ as?: string }>(), { as: 'div' })
</script>

<template>
    <component :is="as" class="jp-float"><slot /></component>
</template>
