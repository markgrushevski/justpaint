<script lang="ts" setup>
/**
 * The sign-in modal — the ONE place an anonymous visitor is asked to
 * authenticate (the owner's ask, 2026-09-18: a modal instead of the bulky block
 * that lived inside the side menu). Mounted once at the app root, so every
 * route can raise it, and opened ONLY through `useAuthGate` — never a local
 * `ref`, because the action that needed the session is waiting on the gate's
 * promise and has to be resumed or released.
 *
 * OriDialog's native <dialog> supplies the focus trap, scroll lock, Esc and
 * ::backdrop dismissal; every one of those paths emits `update:open(false)`,
 * which we settle as "declined" so no caller is left hanging.
 */
import { OriDialog } from '@oriui/vue'
import { useAuthGate } from '@core'
import AuthForm from './AuthForm.vue'

const gate = useAuthGate()

function onOpenChange(open: boolean): void {
    if (!open) gate.settle(false)
}
</script>

<template>
    <OriDialog :open="gate.open" modal title="Sign in" @update:open="onOpenChange">
        <AuthForm :hint="gate.hint" @authenticated="gate.settle(true)" />
    </OriDialog>
</template>
