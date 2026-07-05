<script lang="ts" setup>
/**
 * The slide-in side menu (the legacy pattern the owner liked — DECISIONS
 * 2026-07-04): hidden by default, opened by the hamburger, slides in over a
 * backdrop. Holds auth (login ⇄ register), the profile, and the file actions
 * (the phone-reachable home for New/Load/Save/Export); it REPLACES the old
 * top SessionBar. Saved-drawings list + settings land here later.
 */
import { computed, nextTick, ref, watch } from 'vue'
import { OriAvatar, OriButton, OriField, OriInput, OriTabs } from '@oriui/vue'
import { toApiError, useSessionStore } from '@core'
import ToolIcon from './icons/ToolIcon.vue'

const props = defineProps<{ open: boolean; busy: boolean }>()
const emit = defineEmits<{ close: []; newDrawing: []; load: []; save: []; exportPng: [] }>()

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

// Reset transient form state and move focus into the dialog whenever the menu
// opens — without focus inside, Esc (keydown on the panel tree) never fires.
const panelRef = ref<HTMLElement | null>(null)
watch(
    () => props.open,
    async (open) => {
        if (open) {
            error.value = null
            await nextTick()
            panelRef.value?.focus()
        }
    }
)

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
    } catch (err) {
        error.value = toApiError(err)?.message ?? 'Auth failed (is the server running?).'
    } finally {
        authBusy.value = false
    }
}

async function logout() {
    await session.logout()
}

// File actions: emit the action, then close the drawer (the action runs in the
// host view while the menu slides away).
const fileNew = () => {
    emit('newDrawing')
    emit('close')
}
const fileLoad = () => {
    emit('load')
    emit('close')
}
const fileSave = () => {
    emit('save')
    emit('close')
}
const fileExport = () => {
    emit('exportPng')
    emit('close')
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

                <aside
                    ref="panelRef"
                    class="menu__panel"
                    role="dialog"
                    aria-modal="true"
                    aria-label="Menu"
                    tabindex="-1"
                >
                    <header class="menu__head">
                        <span class="menu__brand">justpaint</span>
                        <button class="menu__close" type="button" aria-label="Close menu" @click="emit('close')">
                            <ToolIcon name="close" />
                        </button>
                    </header>

                    <!-- Profile (signed in) -->
                    <section v-if="session.isLoggedIn" class="menu__section" aria-label="Profile">
                        <div class="menu__profile">
                            <OriAvatar
                                :text="session.user?.displayName ?? session.user?.login ?? '?'"
                                color="primary"
                            />
                            <div class="menu__who">
                                <b class="menu__name">{{ session.user?.displayName ?? session.user?.login }}</b>
                                <span class="menu__login">{{ session.user?.login }}</span>
                            </div>
                        </div>
                        <div class="menu__rating">
                            Rating <b>{{ session.user?.rating }}</b>
                        </div>
                        <OriButton text="Log out" variant="outline" radius="md" @click="logout" />
                    </section>

                    <!-- Auth (anonymous) -->
                    <section v-else class="menu__section" aria-label="Sign in">
                        <OriTabs v-model="authTab" :tabs="AUTH_TABS">
                            <div class="menu__form">
                                <OriField label="Login">
                                    <OriInput
                                        v-model="loginId"
                                        placeholder="email or nickname"
                                        autocomplete="username"
                                        @keyup.enter="submit"
                                    />
                                </OriField>
                                <OriField
                                    label="Password"
                                    :error="error ?? undefined"
                                    hint="Sign in to save and load your drawings."
                                >
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
                    </section>

                    <!-- File actions (the only home for New/Load/Export on phones) -->
                    <section class="menu__section" aria-label="File">
                        <h2 class="menu__section-title">File</h2>
                        <div class="menu__file">
                            <OriButton text="New" variant="outline" radius="md" fluid @click="fileNew" />
                            <OriButton
                                text="Load"
                                variant="outline"
                                radius="md"
                                fluid
                                :loading="props.busy"
                                @click="fileLoad"
                            />
                            <OriButton
                                text="Save"
                                variant="fill"
                                radius="md"
                                fluid
                                :loading="props.busy"
                                @click="fileSave"
                            />
                            <OriButton text="Export" variant="outline" radius="md" fluid @click="fileExport" />
                        </div>
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
    gap: var(--ori-size-gap_lg, 0.75rem);

    width: min(20rem, 85vw);
    padding: var(--ori-size-gap_lg, 0.75rem);
    overflow-y: auto;

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
    gap: var(--ori-size-gap_md, 0.5rem);
}

.menu__section-title {
    margin: 0;

    font-size: var(--ori-font-size_xs, 0.75rem);
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    opacity: 0.6;
}

.menu__profile {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.5rem);
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
    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
    border-radius: var(--ori-size-radius_md, 8px);
    background-color: var(--ori-color-background);

    font-size: var(--ori-font-size_sm, 0.9rem);
}

.menu__form {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.5rem);
    padding-top: var(--ori-size-gap_md, 0.5rem);
}

.menu__file {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--ori-size-gap_sm, 0.25rem);
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
