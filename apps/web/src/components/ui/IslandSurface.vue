<script lang="ts" setup>
/**
 * IslandSurface — floating chrome over the canvas: an `OriSurface` lifted by its shadow alone.
 * When the system asks for more contrast it takes a hairline too (docs/DESIGN-SYSTEM.md).
 */
import { OriSurface } from '@oriui/vue'
import { useThemeStore } from '@core'

withDefaults(
    defineProps<{
        /** The element to render, as for `OriSurface`. */
        as?: string
        elevation?: 'sm' | 'md' | 'lg'
    }>(),
    { as: 'div', elevation: 'md' }
)

const theme = useThemeStore()
</script>

<template>
    <OriSurface :as="as" :bordered="theme.moreContrast" :elevation="elevation">
        <slot />
    </OriSurface>
</template>
