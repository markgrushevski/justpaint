<script lang="ts" setup>
/**
 * Tiny inline SVG icon set (24×24, stroke = currentColor) — no icon-lib
 * dependency. Names cover the editor shell: tools, history, chrome, theme.
 */
export type IconName =
    | 'pen'
    | 'eraser'
    | 'line'
    | 'rect'
    | 'ellipse'
    | 'triangle'
    | 'undo'
    | 'redo'
    | 'menu'
    | 'close'
    | 'layers'
    | 'sun'
    | 'moon'
    | 'monitor'
    | 'help'
    | 'arrow-up'
    | 'arrow-down'
    | 'trash'
    | 'plus'
    | 'minus'
    | 'fit'

const props = defineProps<{ name: IconName }>()

/** Static, handwritten markup — safe for v-html (nothing user-supplied). */
const ICONS: Record<IconName, string> = {
    pen: '<path d="M17 3a2.83 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/>',
    eraser: '<path d="m7 21-4.35-4.35a1 1 0 0 1 0-1.41L13.3 4.6a2 2 0 0 1 2.83 0l3.28 3.28a2 2 0 0 1 0 2.83L11 19"/><path d="M22 21H7"/><path d="m9 8.6 6.4 6.4"/>',
    line: '<path d="M5 19 19 5"/>',
    rect: '<rect x="4" y="6" width="16" height="12" rx="1"/>',
    ellipse: '<ellipse cx="12" cy="12" rx="8.5" ry="6"/>',
    triangle: '<path d="M12 5 20 19H4L12 5z"/>',
    undo: '<polyline points="9 14 4 9 9 4"/><path d="M20 20v-7a4 4 0 0 0-4-4H4"/>',
    redo: '<polyline points="15 14 20 9 15 4"/><path d="M4 20v-7a4 4 0 0 1 4-4h12"/>',
    menu: '<line x1="4" y1="6" x2="20" y2="6"/><line x1="4" y1="12" x2="20" y2="12"/><line x1="4" y1="18" x2="20" y2="18"/>',
    close: '<line x1="6" y1="6" x2="18" y2="18"/><line x1="18" y1="6" x2="6" y2="18"/>',
    layers: '<polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    moon: '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>',
    monitor: '<rect x="2" y="4" width="20" height="13" rx="2"/><path d="M8 21h8m-4-4v4"/>',
    help: '<circle cx="12" cy="12" r="9"/><path d="M9.5 9a2.5 2.5 0 1 1 3.4 2.33c-.7.27-.9.67-.9 1.67"/><circle cx="12" cy="16.5" r="0.5"/>',
    'arrow-up': '<line x1="12" y1="20" x2="12" y2="4"/><polyline points="6 10 12 4 18 10"/>',
    'arrow-down': '<line x1="12" y1="4" x2="12" y2="20"/><polyline points="6 14 12 20 18 14"/>',
    trash: '<polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><line x1="10" y1="11" x2="10" y2="17"/><line x1="14" y1="11" x2="14" y2="17"/>',
    plus: '<line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>',
    minus: '<line x1="5" y1="12" x2="19" y2="12"/>',
    fit: '<path d="M4 8V6a2 2 0 0 1 2-2h2"/><path d="M16 4h2a2 2 0 0 1 2 2v2"/><path d="M20 16v2a2 2 0 0 1-2 2h-2"/><path d="M8 20H6a2 2 0 0 1-2-2v-2"/><rect x="9" y="9.5" width="6" height="5" rx="1"/>'
}
</script>

<template>
    <!-- eslint-disable vue/no-v-html -- static hand-written markup above -->
    <svg
        class="tool-icon"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
        v-html="ICONS[props.name]"
    ></svg>
    <!-- eslint-enable vue/no-v-html -->
</template>

<style scoped>
.tool-icon {
    display: block;
    width: 1.2em;
    height: 1.2em;
}
</style>
