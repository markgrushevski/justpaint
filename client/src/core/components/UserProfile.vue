<script setup lang="ts">
import { mdiAccount, mdiCheck } from '@mdi/js'
import { VAvatar, VButton } from 'vueinjar'
import { useUserStore } from '@core'
import { ref } from 'vue'

const userStore = useUserStore()

const isLoginOpen = ref(false)

function handleLogin() {
    if (!userStore.user) {
        if (isLoginOpen.value) {
        }
    }
}
</script>

<template>
    <div class="user-profile">
        <v-avatar
            v-if="userStore.user"
            :text="userStore.user.nickname"
            :title="userStore.user.nickname"
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
