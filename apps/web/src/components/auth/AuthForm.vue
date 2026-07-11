<script lang="ts" setup>
/**
 * Shared sign-in form (login ⇄ register) — the tabs/fields/submit button,
 * extracted so both SideMenu's Sign in section (/draw) and PlayView's error
 * overlay (/play) can drop it in inline instead of duplicating the markup or
 * punting anonymous /play visitors to /draw.
 */
import { computed, ref } from 'vue'
import { OriButton, OriField, OriInput, OriTabs } from '@oriui/vue'
import { toApiError, useSessionStore } from '@core'

const props = withDefaults(
    defineProps<{
        hint?: string
    }>(),
    {
        hint: 'Sign in to save and load your drawings.'
    }
)
const emit = defineEmits<{
    authenticated: []
}>()

const session = useSessionStore()

const authMode = ref<'login' | 'register'>('login')
const loginId = ref('')
const password = ref('')
const error = ref<string | null>(null)
const authBusy = ref(false)

const AUTH_TABS = [
    { value: 'login', label: 'Log in' },
    { value: 'register', label: 'Register' }
]

// OriTabs models `string | number | undefined` — bridge the narrowed union.
const authTab = computed<string | number | undefined>({
    get: () => authMode.value,
    set: (v) => (authMode.value = v === 'register' ? 'register' : 'login')
})

async function submit() {
    if (authBusy.value) return
    const id = loginId.value.trim()
    if (!id || !password.value) {
        error.value = 'Enter a login and password.'
        return
    }
    error.value = null
    authBusy.value = true
    try {
        if (authMode.value === 'register') await session.register(id, password.value)
        else await session.login(id, password.value)
        password.value = ''
        emit('authenticated')
    } catch (err) {
        error.value = toApiError(err)?.message ?? 'Auth failed (is the server running?).'
    } finally {
        authBusy.value = false
    }
}
</script>

<template>
    <OriTabs v-model="authTab" :tabs="AUTH_TABS">
        <div class="auth-form">
            <OriField label="Login">
                <OriInput
                    v-model="loginId"
                    placeholder="email or nickname"
                    autocomplete="username"
                    @keyup.enter="submit"
                />
            </OriField>
            <OriField label="Password" :error="error ?? undefined" :hint="props.hint">
                <OriInput
                    v-model="password"
                    type="password"
                    placeholder="password"
                    :autocomplete="authMode === 'register' ? 'new-password' : 'current-password'"
                    @keyup.enter="submit"
                />
            </OriField>
            <OriButton
                :text="authMode === 'register' ? 'Create account' : 'Log in'"
                variant="fill"
                radius="md"
                fluid
                :loading="authBusy"
                @click="submit"
            />
        </div>
    </OriTabs>
</template>

<style scoped>
.auth-form {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.5rem);
    padding-top: var(--ori-size-gap_md, 0.5rem);
}
</style>
