<script lang="ts" setup>
import { ref } from 'vue'
import { OriButton } from '@oriui/vue'
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
        <template v-if="session.isLoggedIn">
            <span class="session__who"
                >Signed in as <b>{{ session.user?.login }}</b></span
            >
            <OriButton size="sm" variant="outline" @click="logout">Log out</OriButton>
        </template>
        <template v-else>
            <input
                v-model="loginId"
                class="session__input"
                placeholder="login (email or nickname)"
                autocomplete="username"
                @keyup.enter="submit('login')"
            />
            <input
                v-model="password"
                class="session__input"
                type="password"
                placeholder="password"
                autocomplete="current-password"
                @keyup.enter="submit('login')"
            />
            <OriButton size="sm" variant="fill" :loading="busy" @click="submit('login')">Log in</OriButton>
            <OriButton size="sm" variant="outline" :loading="busy" @click="submit('register')">Register</OriButton>
        </template>
        <span v-if="error" class="session__error">{{ error }}</span>
    </div>
</template>

<style scoped>
.session {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    flex-wrap: wrap;

    padding: 0.5rem 1rem;

    border-bottom: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
    background-color: var(--ori-color-surface, #fafafa);
}

.session__input {
    padding: 0.35rem 0.5rem;

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 20%));
    border-radius: 6px;
}

.session__who {
    font-size: 0.9rem;
}

.session__error {
    color: var(--ori-color-danger, #c0392b);
    font-size: 0.85rem;
}
</style>
