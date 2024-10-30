<script setup lang="ts">
import { computed, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { VButton, VCard } from 'vueinjar'
import { icons, mainAPI, useThemesStore, useArtsStore, CopyArt, useUserStore } from '@core'
import { useDateFormat } from '@vueuse/core'

const usersStore = useUserStore()
const artsStore = useArtsStore()

const refetchArts = ref(false)

const {
    isFetching,
    isError,
    data: arts,
    error
} = useQuery({
    queryKey: ['arts'],
    queryFn: mainAPI.arts.getArts,
    enabled: refetchArts,
    retryDelay: 15000
})
</script>

<template>
    <v-button
        :loading="isFetching"
        :icon="icons.art.load"
        :disabled="refetchArts || !usersStore.isLoggedIn"
        text="Load"
        radius="md"
        variant="tonal"
        fluid
        @click.once="refetchArts = !refetchArts"
    />
    <v-card class="arts" v-if="arts?.length">
        <template #body>
            <div class="art" v-for="art in arts" :key="art.id">
                <div class="art__image grid-bg">
                    <img :src="art" alt="" />
                </div>
                <div class="art__description">
                    <div class="art__title">{{ art.name }}</div>
                    <div class="art__subtitle">created at <br />{{ useDateFormat(art.createdAt, 'YYYY-MM-DD') }}</div>
                </div>
                <v-button variant="tonal" text="Apply" size="sm" radius="md" />
                <CopyArt class="art__action" column />
            </div>
        </template>
    </v-card>
</template>

<style>
.arts.v-card {
    overflow-y: auto;
    padding: 0;
    border-radius: 0;
}

.art {
    display: flex;
    align-items: center;
    gap: calc(var(--v-size-gap));
}

.art__image {
    position: relative;

    width: var(--v-size-action_xl);
    height: var(--v-size-action_xl);

    flex-shrink: 0;
}

.art__description {
    flex-grow: 1;
}

.art__title {
}

.art__subtitle {
    font-size: smaller;
    opacity: 0.8;
}

.art__action {
    margin-left: auto;
}
</style>
