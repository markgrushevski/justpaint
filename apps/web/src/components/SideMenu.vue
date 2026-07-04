<script lang="ts" setup>
/**
 * The slide-in side menu (the legacy pattern the owner liked — DECISIONS
 * 2026-07-04): hidden by default, opened by the hamburger, slides in over a
 * backdrop. Holds auth (login ⇄ register) + the profile; it REPLACES the old
 * top SessionBar. Saved-drawings list + settings land here later.
 */
import { ref, watch } from 'vue'
import { OriButton, OriInput } from '@oriui/vue'
import { toApiError, useSessionStore } from '@core'
import ToolIcon from './icons/ToolIcon.vue'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

const session = useSessionStore()

const authMode = ref<'login' | 'register'>('login')
const loginId = ref('')
const password = ref('')
const error = ref<string | null>(null)
const busy = ref(false)

// Reset transient form state whenever the menu opens fresh.
watch(
    () => props.open,
    (open) => {
        if (open) error.value = null
    }
)

async function submit() {
    if (busy.value) return
    const id = loginId.value.trim()
    if (!id || !password.value) {
        error.value = 'Enter a login and password.'
        return
    }
    error.value = null
    busy.value = true
    try {
        if (authMode.value === 'register') await session.register(id, password.value)
        else await session.login(id, password.value)
        password.value = ''
    } catch (err) {
        error.value = toApiError(err)?.message ?? 'Auth failed (is the server running?).'
    } finally {
        busy.value = false
    }
}

async function logout() {
    await session.logout()
}

function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') emit('close')
}
</script>

<template>
    <Teleport to="body">
        <!-- Explicit duration: Vue then uses a timer, not transitionend — deterministic in hidden tabs/tests. -->
        <Transition name="menu" :duration="240">
            <div v-if="props.open" class="menu" @keydown="onKeydown">
                <div class="menu__backdrop" aria-hidden="true" @click="emit('close')"></div>

                <aside class="menu__panel" role="dialog" aria-modal="true" aria-label="Menu">
                    <header class="menu__head">
                        <span class="menu__brand">justpaint</span>
                        <button class="menu__close" type="button" aria-label="Close menu" @click="emit('close')">
                            <ToolIcon name="close" />
                        </button>
                    </header>

                    <!-- Profile (signed in) -->
                    <section v-if="session.isLoggedIn" class="menu__section" aria-label="Profile">
                        <div class="menu__profile">
                            <span class="menu__avatar" aria-hidden="true">
                                {{
                                    (session.user?.displayName ?? session.user?.login ?? '?').slice(0, 1).toUpperCase()
                                }}
                            </span>
                            <div class="menu__who">
                                <b class="menu__name">{{ session.user?.displayName ?? session.user?.login }}</b>
                                <span class="menu__login">{{ session.user?.login }}</span>
                            </div>
                        </div>
                        <div class="menu__rating">
                            Rating <b>{{ session.user?.rating }}</b>
                        </div>
                        <OriButton variant="outline" @click="logout">Log out</OriButton>
                    </section>

                    <!-- Auth (anonymous) -->
                    <section v-else class="menu__section" aria-label="Sign in">
                        <div class="menu__tabs" role="tablist" aria-label="Auth mode">
                            <button
                                class="menu__tab"
                                :class="{ 'menu__tab--active': authMode === 'login' }"
                                role="tab"
                                :aria-selected="authMode === 'login'"
                                type="button"
                                @click="authMode = 'login'"
                            >
                                Log in
                            </button>
                            <button
                                class="menu__tab"
                                :class="{ 'menu__tab--active': authMode === 'register' }"
                                role="tab"
                                :aria-selected="authMode === 'register'"
                                type="button"
                                @click="authMode = 'register'"
                            >
                                Register
                            </button>
                        </div>

                        <OriInput
                            v-model="loginId"
                            placeholder="login (email or nickname)"
                            autocomplete="username"
                            aria-label="Login"
                            @keyup.enter="submit"
                        />
                        <OriInput
                            v-model="password"
                            type="password"
                            placeholder="password"
                            :autocomplete="authMode === 'register' ? 'new-password' : 'current-password'"
                            aria-label="Password"
                            @keyup.enter="submit"
                        />
                        <OriButton variant="fill" :loading="busy" @click="submit">
                            {{ authMode === 'register' ? 'Create account' : 'Log in' }}
                        </OriButton>
                        <p v-if="error" class="menu__error" role="alert">{{ error }}</p>
                        <p class="menu__hint">Sign in to save and load your drawings.</p>
                    </section>
                </aside>
            </div>
        </Transition>
    </Teleport>
</template>

<style scoped>
.menu {
    position: fixed;
    inset: 0;
    z-index: 100;
}

.menu__backdrop {
    position: absolute;
    inset: 0;
    background-color: rgb(0 0 0 / 35%);
}

.menu__panel {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_lg, 1rem);

    width: min(20rem, 85vw);
    padding: var(--ori-size-gap_lg, 1rem);

    background-color: var(--ori-color-surface);
    color: var(--ori-color-on-surface);
    box-shadow: 8px 0 32px rgb(0 0 0 / 18%);
}

.menu__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
}

.menu__brand {
    font-weight: 700;
    font-size: var(--ori-font-size_lg, 1.15rem);
    color: var(--ori-color-primary);
    letter-spacing: -0.01em;
}

.menu__close {
    display: grid;
    place-items: center;

    width: 2.2rem;
    height: 2.2rem;

    border: none;
    border-radius: var(--ori-size-radius_md, 8px);
    background: transparent;
    color: var(--ori-color-on-surface);

    cursor: pointer;
}

.menu__close:hover {
    background-color: color-mix(in srgb, var(--ori-color-on-surface) 8%, transparent);
}

.menu__section {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.75rem);
}

.menu__profile {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.75rem);
}

.menu__avatar {
    display: grid;
    place-items: center;

    width: 2.75rem;
    height: 2.75rem;

    border-radius: 50%;
    background-color: var(--ori-color-primary);
    color: var(--ori-color-on-primary);

    font-weight: 700;
    font-size: 1.2rem;
}

.menu__who {
    display: flex;
    flex-direction: column;
    min-width: 0;
}

.menu__name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.menu__login {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;

    font-size: var(--ori-font-size_sm, 0.85rem);
    opacity: 0.7;
}

.menu__rating {
    padding: var(--ori-size-gap_sm, 0.5rem) var(--ori-size-gap_md, 0.75rem);

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
    border-radius: var(--ori-size-radius_md, 8px);
    background-color: var(--ori-color-background);

    font-size: var(--ori-font-size_sm, 0.9rem);
}

.menu__tabs {
    display: flex;

    padding: 0.2rem;
    gap: 0.2rem;

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
    border-radius: var(--ori-size-radius_md, 8px);
}

.menu__tab {
    flex: 1 1 0;

    padding: 0.4rem 0;

    border: none;
    border-radius: calc(var(--ori-size-radius_md, 8px) - 3px);
    background: transparent;
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.9rem);
    cursor: pointer;
}

.menu__tab--active {
    background-color: var(--ori-color-primary);
    color: var(--ori-color-on-primary);
    font-weight: 600;
}

.menu__error {
    margin: 0;
    color: var(--ori-color-danger, #c0392b);
    font-size: var(--ori-font-size_sm, 0.85rem);
}

.menu__hint {
    margin: 0;
    font-size: var(--ori-font-size_sm, 0.85rem);
    opacity: 0.65;
}

/* Slide + fade (the legacy transform pattern). */
.menu-enter-active,
.menu-leave-active {
    transition: opacity 200ms ease;
}

.menu-enter-active .menu__panel,
.menu-leave-active .menu__panel {
    transition: transform 220ms ease;
}

.menu-enter-from,
.menu-leave-to {
    opacity: 0;
}

.menu-enter-from .menu__panel,
.menu-leave-to .menu__panel {
    transform: translateX(-100%);
}
</style>
