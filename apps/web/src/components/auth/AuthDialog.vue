<script lang="ts" setup>
/**
 * The sign-in modal — the one place an anonymous visitor authenticates.
 * Mounted once at the app root so every route can raise it, opened only
 * through `useAuthGate` since the action that needed the session is waiting
 * on the gate's promise and must be resumed or released.
 *
 * `v-if` on the form matters: `OriDialog` keeps its <dialog> in the DOM
 * whether open or not, so without it a typed-and-abandoned password, or a
 * failed attempt's error, would still be sitting there next time the gate
 * raises the dialog for something else.
 */
import { OriDialog } from '@oriui/vue'
import { useAuthGate } from '@core'
import AuthForm from './AuthForm.vue'

const gate = useAuthGate()

// Esc, the backdrop and the x all arrive here; settle them as "declined" so no
// caller is left hanging.
function onOpenChange(open: boolean): void {
    if (!open) gate.settle(false)
}
</script>

<template>
    <OriDialog :open="gate.open" modal title="Sign in" @update:open="onOpenChange">
        <AuthForm v-if="gate.open" :hint="gate.hint" @authenticated="gate.settle(true)" />
    </OriDialog>
</template>
