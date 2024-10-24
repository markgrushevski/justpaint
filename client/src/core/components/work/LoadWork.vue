<script setup lang="ts">
import { computed, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { VButton, VCard } from 'vueinjar'
import { icons, mainAPI, useThemesStore, useWorksStore, CopyWork } from '@core'

const themesStore = useThemesStore()
const worksStore = useWorksStore()

const refetch = ref(false)

const {
    isFetching,
    isError,
    data: works,
    error
} = useQuery({
    queryKey: ['works'],
    queryFn: () => {
        console.log('useQuery')
        return mainAPI.works.getWorks()
    },
    enabled: refetch
})
</script>

<template>
    <v-button
        :loading="isFetching"
        :icon="icons.work.load"
        :disabled="refetch"
        text="Load"
        radius="md"
        variant="tonal"
        fluid
        @click.once="refetch = !refetch"
    />
    <v-card class="works" v-if="works?.length">
        <template #body>
            <div class="work" v-for="work in works" :key="work.id">
                <div class="work__image grid-bg"></div>
                <div class="work__description">
                    <div class="work__title">{{ work.name }}</div>
                    <div class="work__subtitle">created at {{ work.createdAt }}</div>
                </div>
                <v-button variant="tonal" text="Apply" size="sm" />
                <CopyWork :fluid="false" class="work__action" variant="text" size="md" text="" />
            </div>
        </template>
    </v-card>
</template>

<style>
.works.v-card {
    overflow-y: auto;
    padding: 0;
    border-radius: 0;
}

.work {
    display: flex;
    align-items: center;
    gap: calc(var(--v-size-gap) * 2);
}

.work__image {
    width: 48px;
    height: 48px;

    flex-shrink: 0;
}

.work__title {
}

.work__subtitle {
    font-size: smaller;
    opacity: 0.8;
}

.work__action {
    margin-left: auto;
}
</style>
