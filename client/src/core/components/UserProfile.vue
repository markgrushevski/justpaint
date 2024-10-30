<script setup lang="ts">
import { compile, computed, onBeforeMount, onMounted, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { mdiAccount, mdiCheck } from '@mdi/js'
import { VAvatar, VButton, VIcon } from 'vueinjar'
import { type Art, icons, mainAPI, useUserStore } from '@core'

const userStore = useUserStore()

const loading = ref(false)
const formAction = ref<'registration' | 'login' | 'recover'>('login')

const isLogin = computed(() => formAction.value === 'login')
const isRegister = computed(() => formAction.value === 'registration')
const isRecover = computed(() => formAction.value === 'recover')

function toggleLogin() {
    if (isLogin.value) formAction.value = 'registration'
    else if (isRegister.value) formAction.value = 'login'
}

async function handleFormAction() {
    try {
        loading.value = true

        switch (formAction.value) {
            case 'registration':
                await handleRegister()
                break
            case 'login':
                await handleLogin()
                break
            case 'recover':
                await handleRecover()
                break
        }
    } catch (e) {
        console.error(e)
    } finally {
        loading.value = false
    }
}

async function handleRegister() {
    const success = await mainAPI.auth.register(userStore.formData)
    if (success) {
        userStore.isLoggedIn = true
    }
}

async function handleLogin() {
    const success = await mainAPI.auth.login(userStore.formData)
    if (success) {
        userStore.isLoggedIn = true
    }
}

async function handleRecover() {}

async function handleLogout() {
    const success = await mainAPI.auth.logout()
    if (success) {
        userStore.$reset()
    }
}

async function initUser() {
    try {
        loading.value = true

        const nickname = await mainAPI.users.getNickname()

        if (nickname) {
            userStore.isLoggedIn = true
            userStore.formData.nickname = nickname
        }
    } catch (e) {
        userStore.isLoggedIn = false
    } finally {
        loading.value = false
    }
}

const { data: arts } = useQuery<Art[]>({
    queryKey: ['arts'],
    enabled: false
})

onBeforeMount(() => {
    initUser()
})
</script>

<template>
    <div class="user-profile">
        <template v-if="userStore.isLoggedIn">
            <v-avatar
                :text="userStore.formData.nickname"
                :title="userStore.formData.nickname"
                :subtitle="arts?.length ? `Arts ${arts?.length}` : ''"
                class="user-avatar"
            />
            <v-button :icon="icons.auth.logout" style="align-self: center" variant="text" @click="handleLogout" />
        </template>
        <template v-else>
            <!--<v-button :icon="mdiAccount" size="lg" variant="text" spaced />-->
            <v-icon :icon="mdiAccount" size="sm" spaced />
            <form class="user-login" @submit.prevent="handleFormAction">
                <fieldset>
                    <legend>{{ formAction }}</legend>
                    <label>
                        <span>nickname</span>
                        <input v-model="userStore.formData.nickname" type="text" autocomplete="on" />
                    </label>
                    <label>
                        <span>password</span>
                        <input v-model="userStore.formData.password" type="password" autocomplete="on" />
                    </label>
                    <div class="user-login__actions">
                        <v-button
                            :text="isLogin ? 'To registration' : 'To sign in'"
                            variant="text"
                            size="xs"
                            type="button"
                            @click="toggleLogin"
                        />
                        <v-button :loading="loading" text="Submit" variant="text" size="sm" fluid type="submit" />
                    </div>
                </fieldset>
            </form>
        </template>
    </div>
</template>

<style>
.user-profile {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--v-size-gap);
}

.user-profile > i {
    flex-shrink: 0;
}

.user-login {
    display: flex;
    flex-direction: column;
    gap: var(--v-size-gap);
}

.user-login legend {
    font-size: 1.125em;
    text-transform: capitalize;
}

.user-login legend,
.user-login label {
    line-height: 1.5;
}

.user-login input {
    width: 100%;
}

.user-login__actions {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
}

.user-login__actions button[type='submit'] {
    text-transform: uppercase;
}
</style>
