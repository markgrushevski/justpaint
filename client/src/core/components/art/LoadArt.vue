<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { VButton, VCard } from 'vueinjar'
import { icons, mainAPI, useArtsStore, CopyArt, useUserStore, type Art } from '@core'
import { useDateFormat } from '@vueuse/core'
import { onMounted, ref, watch } from 'vue'
import { getCanvasDataURL } from '@modules/canvas'

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

async function getCompositedArts(arts: Art[]): Promise<Art[]> {
    console.log({ toComposite: arts })

    return Promise.all(
        arts.map(async (art) => {
            return {
                ...art,
                dataURL: await getCompositedArtLayers(art)
            }
        })
    )
}

async function getCompositedArtLayers(art: Art): Promise<string> {
    const canvas = document.createElement('canvas')
    const ctx = canvas.getContext('2d')!

    for (const layer of art.layers) {
        await new Promise((resolve) => {
            if (layer.dataURL) {
                const image = new Image()
                image.onload = () => {
                    ctx.drawImage(image, 0, 0, canvas.width, canvas.height)
                    resolve(true)
                }
                image.src = layer.dataURL
            }
        })
    }

    return getCanvasDataURL(canvas)
}

function handleCopyCompositedArt() {}

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
        :loading="isFetching"
        :icon="icons.art.load"
        :disabled="!usersStore.isLoggedIn"
        text="Load"
        radius="md"
        variant="tonal"
        fluid
        @click="refetch"
    />
    <v-card class="arts" v-if="arts?.length">
        <template #body>
            <div class="art" v-for="art in arts" :key="art.id">
                <div class="art__image grid-bg">
                    <img :src="art.dataURL" alt="" />
                </div>
                <div class="art__description">
                    <div class="art__title">{{ art.name }}</div>
                    <div class="art__subtitle">created at <br />{{ useDateFormat(art.createdAt, 'YYYY-MM-DD') }}</div>
                </div>
                <v-button variant="tonal" text="Apply" size="sm" radius="md" />
                <CopyArt class="art__action" column @copyArt="handleCopyCompositedArt" />
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

.art__image img {
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
