<script lang="ts" setup>
import { useThemeStore } from '@core'
import AuthDialog from './components/auth/AuthDialog.vue'

// Construct the theme controller once at the app root so the persisted / OS
// theme is applied on EVERY route. Without this, a direct load of (or refresh
// on) any route other than /draw would render the default light theme until
// DrawView happened to construct the store. Pinia stores are singletons, so
// DrawView's own `useThemeStore()` reuses this same instance.
useThemeStore()
</script>

<template>
    <RouterView />

    <!-- The sign-in modal lives here, not inside a view: any route can ask for a
         session, and here it has no `pointer-events: none` ancestor to inherit
         from (the trap docs/NOTES.md records about the editor's overlay layer). -->
    <AuthDialog />
</template>
