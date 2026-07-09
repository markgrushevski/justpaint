<script lang="ts" setup>
import { useThemeStore } from '@core'

// Construct the theme controller once at the app root so the persisted / OS
// theme is applied on EVERY route. Without this, a direct load of (or refresh
// on) any route other than /draw would render the default light theme until
// DrawView happened to construct the store. Pinia stores are singletons, so
// DrawView's own `useThemeStore()` reuses this same instance.
useThemeStore()
</script>

<template>
    <RouterView />
</template>
