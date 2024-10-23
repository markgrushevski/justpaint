<script setup lang="ts">
import { mdiAccount, mdiCheck } from '@mdi/js'
import { VAvatar, VButton } from 'vueinjar'
import { useAuthStore } from '@core'
import { ref } from 'vue'

const authStore = useAuthStore()

const isLoginOpen = ref(false)

function handleLogin() {
    if (!authStore.user) {
        if (isLoginOpen.value) {
        }
    }
}
</script>

<template>
    <div class="user-profile">
        <v-avatar
            v-if="authStore.user"
            :text="authStore.user.nickname"
            :title="authStore.user.nickname"
            :subtitle="`works ${0}`"
            class="user-avatar"
        />
        <template v-else>
            <v-button :icon="isLoginOpen ? mdiCheck : mdiAccount" size="lg" variant="text" @click="handleLogin" />
            <form class="user-login">
                <label>
                    <span>nickname</span>
                    <input />
                </label>
                <label>
                    <span>password</span>
                    <input />
                </label>
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

.user-login {
    display: flex;
    flex-direction: column;
    gap: var(--v-size-gap);
}
</style>
