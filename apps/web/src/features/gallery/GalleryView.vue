<script lang="ts" setup>
/**
 * "My drawings" (`/gallery`): the signed-in visitor's saved free drawings as a grid of
 * previews, newest first, with rename and delete. Each preview is rendered in the browser
 * from its document (`useThumbnail`).
 */
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { OriButton, OriCard, OriIcon, OriSkeleton, OriToaster, useToast } from '@oriui/vue'
import {
    icons,
    toApiError,
    useAuthGate,
    useDeleteDrawing,
    useDrawingsList,
    useRenameDrawing,
    useSessionStore
} from '@core'
import type { DrawingMeta } from '@core'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import ModeNav from '../../components/ModeNav.vue'
import DrawingCard from './DrawingCard.vue'
import RenameDialog from './RenameDialog.vue'

/** Toast durations, ms. */
const TOAST = { success: 3500, error: 8000 } as const

const SKELETON_CARDS = 6

const session = useSessionStore()
const gate = useAuthGate()
const toaster = useToast()

// Anonymous is only known once the cookie restore settles; until then the page is loading.
const restored = ref(false)
session.ready().then(() => {
    restored.value = true
})

const {
    data,
    error,
    isPending,
    isError,
    isFetching,
    hasNextPage,
    isFetchingNextPage,
    isFetchNextPageError,
    fetchNextPage,
    refetch
} = useDrawingsList(() => session.user?.id)

const drawings = computed<DrawingMeta[]>(() => data.value?.pages.flatMap((page) => page.drawings) ?? [])

type Section = 'loading' | 'signin' | 'error' | 'empty' | 'grid'

const section = computed<Section>(() => {
    if (!restored.value) return 'loading'
    if (!session.isLoggedIn) return 'signin'
    if (isPending.value) return 'loading'
    if (isError.value && !drawings.value.length) return 'error'
    return drawings.value.length ? 'grid' : 'empty'
})

const errorMessage = computed(() => toApiError(error.value)?.message ?? 'Could not load your drawings.')

function signIn(): void {
    gate.ensure('Sign in to see your drawings.')
}

function failure(err: unknown): string {
    return err instanceof Error ? err.message : 'Something went wrong.'
}

// The targets outlive their dialogs' `open` flags so the closing dialog keeps its text.
const renameMutation = useRenameDrawing()
const renameOpen = ref(false)
const renameTarget = ref<DrawingMeta | null>(null)

function askRename(drawing: DrawingMeta): void {
    renameTarget.value = drawing
    renameOpen.value = true
}

function saveRename(name: string): void {
    const target = renameTarget.value
    if (!target) return
    renameMutation.mutate(
        { id: target.id, name },
        {
            onSuccess: () => {
                renameOpen.value = false
                toaster.success({ text: 'Renamed.', duration: TOAST.success })
            },
            onError: (err) => toaster.error({ text: `Could not rename: ${failure(err)}`, duration: TOAST.error })
        }
    )
}

const deleteMutation = useDeleteDrawing()
const deleteOpen = ref(false)
const deleteTarget = ref<DrawingMeta | null>(null)

function askDelete(drawing: DrawingMeta): void {
    deleteTarget.value = drawing
    deleteOpen.value = true
}

function confirmDelete(): void {
    const target = deleteTarget.value
    if (!target || deleteMutation.isPending.value) return
    deleteMutation.mutate(target.id, {
        onSuccess: () => toaster.success({ text: 'Deleted.', duration: TOAST.success }),
        onError: (err) => toaster.error({ text: `Could not delete: ${failure(err)}`, duration: TOAST.error }),
        onSettled: () => {
            deleteOpen.value = false
        }
    })
}
</script>

<template>
    <div class="gallery">
        <ModeNav class="gallery__nav" />

        <main class="gallery__content" aria-labelledby="gallery-title">
            <header class="gallery__header">
                <div class="gallery__heading">
                    <h1 id="gallery-title" class="gallery__title">My drawings</h1>
                    <p class="gallery__subtitle">Your saved sketches, newest first.</p>
                </div>
                <OriButton
                    :as="RouterLink"
                    to="/draw"
                    label="New drawing"
                    variant="solid"
                    color="primary"
                    radius="md"
                    :icon="icons.mdiPlus"
                    icon-position="left"
                />
            </header>

            <template v-if="section === 'loading'">
                <p class="gallery__sr-only" role="status">Loading your drawings…</p>
                <div class="gallery__grid" aria-hidden="true">
                    <OriCard v-for="n in SKELETON_CARDS" :key="n" class="gallery__placeholder">
                        <OriSkeleton class="gallery__placeholder-thumb" radius="none" />
                        <div class="gallery__placeholder-meta">
                            <OriSkeleton class="gallery__placeholder-name" />
                            <div class="gallery__placeholder-time">
                                <OriSkeleton class="gallery__placeholder-time-line" />
                            </div>
                        </div>
                    </OriCard>
                </div>
            </template>

            <OriCard v-else-if="section === 'signin'" class="gallery__state">
                <div class="gallery__state-body">
                    <OriIcon class="gallery__state-icon" :icon="icons.mdiImageMultipleOutline" size="xl" />
                    <h2 class="gallery__state-title">Sign in to see your saved drawings</h2>
                    <OriButton label="Sign in" variant="solid" radius="md" @click="signIn" />
                </div>
            </OriCard>

            <OriCard v-else-if="section === 'error'" class="gallery__state">
                <div class="gallery__state-body">
                    <p class="gallery__state-title" role="alert">{{ errorMessage }}</p>
                    <OriButton
                        label="Try again"
                        variant="outline"
                        color="surface"
                        radius="md"
                        :loading="isFetching"
                        @click="() => refetch()"
                    />
                </div>
            </OriCard>

            <OriCard v-else-if="section === 'empty'" class="gallery__state">
                <div class="gallery__state-body">
                    <OriIcon class="gallery__state-icon" :icon="icons.mdiImageMultipleOutline" size="xl" />
                    <h2 class="gallery__state-title">No drawings yet</h2>
                    <OriButton :as="RouterLink" to="/draw" label="Start drawing" variant="solid" radius="md" />
                </div>
            </OriCard>

            <template v-else>
                <ul class="gallery__grid">
                    <li v-for="drawing in drawings" :key="drawing.id" class="gallery__item">
                        <DrawingCard :drawing="drawing" @rename="askRename" @remove="askDelete" />
                    </li>
                </ul>

                <div v-if="hasNextPage || isFetchNextPageError" class="gallery__more">
                    <p v-if="isFetchNextPageError" class="gallery__note" role="alert">Could not load more drawings.</p>
                    <OriButton
                        :label="isFetchNextPageError ? 'Try again' : 'Load more'"
                        variant="outline"
                        color="surface"
                        radius="md"
                        :loading="isFetchingNextPage"
                        @click="() => fetchNextPage()"
                    />
                </div>
            </template>
        </main>

        <RenameDialog
            :open="renameOpen"
            :name="renameTarget?.name ?? ''"
            :busy="renameMutation.isPending.value"
            @save="saveRename"
            @cancel="renameOpen = false"
        />

        <ConfirmDialog
            :open="deleteOpen"
            title="Delete this drawing?"
            :message="`“${deleteTarget?.name ?? ''}” will be gone for good.`"
            confirm-text="Delete"
            cancel-text="Cancel"
            danger
            @confirm="confirmDelete"
            @cancel="deleteOpen = false"
        />

        <OriToaster position="top-center" align="center" />
    </div>
</template>

<style scoped>
.gallery {
    height: 100%;
    overflow-y: auto;

    background-color: var(--ori-color-background);
    color: var(--ori-color-on-background);
}

.gallery__nav {
    position: fixed;
    top: var(--ori-size-gap_md, 0.5rem);
    left: var(--ori-size-gap_md, 0.5rem);
    z-index: 10;
}

.gallery__content {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_xxl, 1.5rem);

    max-width: 1100px;
    margin: 0 auto;
    /* The top clears the fixed mode switcher. */
    padding: 4.5rem 16px 3rem;
}

.gallery__header {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--ori-size-gap_lg, 0.75rem);
}

.gallery__heading {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_xs, 0.125rem);
}

.gallery__title {
    margin: 0;

    font-size: var(--ori-font-size_xl, 1.4rem);
    font-weight: 800;
    letter-spacing: -0.01em;
}

.gallery__subtitle {
    margin: 0;

    font-size: var(--ori-font-size_sm, 0.875rem);
    /* 0.7 keeps the muted line past WCAG AA. */
    opacity: var(--jp-dim, 0.7);
}

.gallery__grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: var(--ori-size-gap_xl, 1rem);

    margin: 0;
    padding: 0;
    list-style: none;
}

@media (width <= 600px) {
    .gallery__grid {
        grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    }
}

.gallery__item {
    min-width: 0;
}

/* The same shape as a drawing card: a full-bleed 4:3 block over a name and a time row. */
.gallery__placeholder {
    --ori-card-padding: 0;

    display: flex;
    flex-direction: column;

    min-width: 0;
}

.gallery__placeholder-thumb {
    aspect-ratio: 4 / 3;
}

.gallery__placeholder-meta {
    display: flex;
    flex-direction: column;

    padding: var(--ori-size-gap_lg) var(--ori-size-gap_xl) var(--ori-size-gap_md);
}

.gallery__placeholder-name {
    width: 60%;
    height: 1rem;
}

.gallery__placeholder-time {
    display: flex;
    align-items: center;

    min-height: var(--ori-size-action_md);
}

.gallery__placeholder-time-line {
    width: 30%;
    height: 0.75rem;
}

.gallery__state {
    width: min(26rem, 100%);
    margin: var(--ori-size-gap_xxl, 1.5rem) auto 0;
}

.gallery__state-body {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_lg, 0.75rem);

    text-align: center;
}

.gallery__state-icon {
    opacity: 0.6;
}

.gallery__state-title {
    margin: 0;

    font-size: var(--ori-font-size_lg, 1.125rem);
    font-weight: 700;
}

.gallery__more {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.5rem);
}

.gallery__note {
    margin: 0;

    color: var(--ori-color-danger-text, var(--ori-color-danger));
    font-size: var(--ori-font-size_sm, 0.875rem);
}

.gallery__sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
    border: 0;
}
</style>
