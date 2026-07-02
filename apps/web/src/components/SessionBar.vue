<script lang="ts" setup>
import { ref } from 'vue'
import { OriButton, OriInput } from '@oriui/vue'
import { toApiError, useSessionStore } from '@core'

const session = useSessionStore()

const loginId = ref('')
const password = ref('')
const error = ref<string | null>(null)
const busy = ref(false)

async function submit(kind: 'login' | 'register') {
    if (busy.value) return
    const id = loginId.value.trim()
    if (!id || !password.value) {
        error.value = 'Enter a login and password.'
        return
    }
    error.value = null
    busy.value = true
    try {
        if (kind === 'register') await session.register(id, password.value)
        else await session.login(id, password.value)
        password.value = ''
    } catch (err) {
        error.value = toApiError(err)?.message ?? 'Auth failed (is the Go server running?).'
    } finally {
        busy.value = false
    }
}

async function logout() {
    await session.logout()
}
</script>

<template>
    <div class="session">
        <span class="session__brand">justpaint</span>
        <template v-if="session.isLoggedIn">
            <span class="session__who"
                >Signed in as <b>{{ session.user?.login }}</b></span
            >
            <OriButton size="sm" variant="outline" @click="logout">Log out</OriButton>
        </template>
        <template v-else>
            <OriInput
                v-model="loginId"
                class="session__input"
                size="sm"
                placeholder="login (email or nickname)"
                autocomplete="username"
                aria-label="Login"
                @keyup.enter="submit('login')"
            />
            <OriInput
                v-model="password"
                class="session__input"
                size="sm"
                type="password"
                placeholder="password"
                autocomplete="current-password"
                aria-label="Password"
                @keyup.enter="submit('login')"
            />
            <OriButton size="sm" variant="fill" :loading="busy" @click="submit('login')">Log in</OriButton>
            <OriButton size="sm" variant="outline" :loading="busy" @click="submit('register')">Register</OriButton>
        </template>
        <span v-if="error" class="session__error" role="alert">{{ error }}</span>
    </div>
</template>

<style scoped>
.session {
    display: flex;
    gap: var(--ori-size-gap_md, 0.5rem);
    align-items: center;
    flex-wrap: wrap;

    padding: var(--ori-size-gap_md, 0.5rem) var(--ori-size-gap_lg, 1rem);

    border-bottom: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
    background-color: var(--ori-color-surface);
}

.session__brand {
    margin-right: auto;

    font-weight: 700;
    font-size: var(--ori-font-size_lg, 1.1rem);
    color: var(--ori-color-primary);
    letter-spacing: -0.01em;
}

.session__who {
    font-size: var(--ori-font-size_sm, 0.9rem);
    color: var(--ori-color-on-surface);
}

.session__input {
    width: 12rem;
    max-width: 40vw;
}

.session__error {
    flex-basis: 100%;
    color: var(--ori-color-danger);
    font-size: var(--ori-font-size_sm, 0.85rem);
}
</style>
