<script lang="ts" setup>
import { ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { useDateFormat } from '@vueuse/core'
import { VButton, VCard } from 'vueinjar'
import { type Art, CopyArt, icons, mainAPI, useArtsStore, useUserStore } from '@core'
import { getCompositedArts } from '@modules/canvas'

const {
    isFetching,
    isError,
    data: fetchedArts,
    error,
    refetch
} = useQuery({
    queryKey: ['arts'],
    queryFn: mainAPI.arts.getArts,
    enabled: false
})

const usersStore = useUserStore()
const artsStore = useArtsStore()

const arts = ref<Art[]>()

/* async function handleCopyCompositedArt(target: 'text' | 'image', art: Art) {
    const dataURL = art.dataURL ?? ''
    if (target === 'text') {
        await copyToClipboard(target, dataURL ?? '')
    } else if (target === 'image') {
        const blob = await (await fetch(dataURL)).blob()
        await copyToClipboard(target, blob)
    }
} */

watch(
    fetchedArts,
    async () => {
        arts.value = await getCompositedArts(fetchedArts.value ?? [])
        console.log({ compositedArts: arts.value })
    },
    { deep: true }
)
</script>

<template>
    <v-button
        :disabled="!usersStore.isLoggedIn"
        :icon="icons.art.load"
        :loading="isFetching"
        fluid
        radius="md"
        text="Load"
        variant="tonal"
        @click="refetch"
    />
    <v-card v-if="arts?.length" class="arts">
        <template #body>
            <div v-for="art in arts" :key="art.id" class="art">
                <div class="art__image grid-bg">
                    <img :src="art.dataURL" alt="" />
                </div>
                <div class="art__description">
                    <div class="art__title">{{ art.name }}</div>
                    <div class="art__subtitle">created at <br />{{ useDateFormat(art.createdAt, 'YYYY-MM-DD') }}</div>
                </div>
                <v-button radius="md" size="sm" text="Apply" variant="tonal" />
                <CopyArt :data-url="art.dataURL" class="art__action" column />
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
    width: var(--v-size-action_xl);
    height: var(--v-size-action_xl);

    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
}

.art__image > * {
    max-width: 100%;
    max-height: 100%;
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
