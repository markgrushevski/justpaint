<script lang="ts" setup>
/**
 * One saved drawing: its preview and name open it in the editor, and a menu carries the
 * other actions. The menu sits beside the lifting card, never inside it: a transformed
 * ancestor becomes the containing block of the menu's fixed, anchored panel and misplaces it.
 */
import { computed } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { OriButton, OriCard, OriIcon, OriMenu, OriSkeleton } from '@oriui/vue'
import type { MenuItem } from '@oriui/headless/vue'
import { icons } from '@core'
import type { DrawingMeta } from '@core'
import { timeAgo } from './relativeTime'
import { THUMB_HEIGHT, THUMB_WIDTH } from './thumbnailCrop'
import { useThumbnail } from './useThumbnail'

const props = defineProps<{ drawing: DrawingMeta }>()
const emit = defineEmits<{ rename: [drawing: DrawingMeta]; remove: [drawing: DrawingMeta] }>()

const router = useRouter()
const { src, paper, failed } = useThumbnail(() => props.drawing.id)

const editorRoute = computed(() => ({ path: '/draw', query: { id: props.drawing.id } }))
const updated = computed(() => timeAgo(props.drawing.updatedAt))
const updatedExact = computed(() => new Date(props.drawing.updatedAt).toLocaleString())

const ITEMS: MenuItem[] = [
    { value: 'open', label: 'Open' },
    { value: 'rename', label: 'Rename' },
    { value: 'sep', separator: true },
    { value: 'delete', label: 'Delete' }
]

const ITEM_ICONS: Record<string, string> = {
    open: icons.mdiPencil,
    rename: icons.mdiRename,
    delete: icons.mdiTrashCanOutline
}

function onSelect(value: string): void {
    if (value === 'open') router.push(editorRoute.value)
    else if (value === 'rename') emit('rename', props.drawing)
    else if (value === 'delete') emit('remove', props.drawing)
}
</script>

<template>
    <article class="drawing">
        <div class="drawing__lift">
            <OriCard class="drawing__card">
                <RouterLink class="drawing__link" :to="editorRoute">
                    <div class="drawing__thumb" :style="{ backgroundColor: paper }">
                        <img
                            v-if="src"
                            class="drawing__img"
                            :src="src"
                            :alt="`Preview of ${drawing.name}`"
                            :width="THUMB_WIDTH"
                            :height="THUMB_HEIGHT"
                        />
                        <OriIcon
                            v-else-if="failed"
                            class="drawing__missing"
                            :icon="icons.mdiImageMultipleOutline"
                            size="xl"
                        />
                        <OriSkeleton v-else class="drawing__skeleton" radius="none" />
                    </div>

                    <div class="drawing__meta">
                        <span class="drawing__name">{{ drawing.name }}</span>
                        <time class="drawing__time" :datetime="drawing.updatedAt" :title="updatedExact">{{
                            updated
                        }}</time>
                    </div>
                </RouterLink>
            </OriCard>
        </div>

        <div class="drawing__actions">
            <OriMenu :items="ITEMS" placement="bottom-end" @select="onSelect">
                <template #trigger="{ props: trigger }">
                    <span class="drawing__trigger">
                        <OriButton
                            v-bind="trigger"
                            variant="text"
                            color="surface"
                            radius="md"
                            :icon="icons.mdiDotsHorizontal"
                            :aria-label="`Actions for ${drawing.name}`"
                        />
                    </span>
                </template>
                <template #item="{ item }">
                    <span class="drawing__item" :class="{ 'drawing__item--danger': item.value === 'delete' }">
                        <OriIcon :icon="ITEM_ICONS[item.value]" />
                        {{ item.label }}
                    </span>
                </template>
            </OriMenu>
        </div>
    </article>
</template>

<style scoped>
.drawing {
    /* The meta block's insets; the menu button is placed from the same numbers. */
    --drawing-inset-x: var(--ori-size-gap_xl);
    --drawing-inset-bottom: var(--ori-size-gap_md);

    position: relative;
}

/* The lift is on this wrapper, not the OriCard, so the card keeps its own transitions. */
.drawing__lift {
    border-radius: var(--ori-size-radius_lg);
    transition:
        transform 160ms ease,
        box-shadow 160ms ease;
}

.drawing:hover .drawing__lift,
.drawing:focus-within .drawing__lift {
    transform: translateY(-2px);
    box-shadow: var(--ori-shadow-md);
}

/* The preview runs to the card's edges, so the card gives up its padding; the meta block pads itself.
   The grid track, not the name's length, sets the card's width. */
.drawing__card {
    --ori-card-padding: 0;

    min-width: 0;
}

.drawing__link {
    display: flex;
    flex-direction: column;

    color: inherit;
    text-decoration: none;
}

/* The global focus ring covers buttons and inputs, not links. */
.drawing__link:focus-visible {
    outline: 2px solid var(--ori-color-primary);
    outline-offset: 2px;
}

.drawing__thumb {
    position: relative;
    display: grid;
    place-items: center;

    aspect-ratio: 4 / 3;
    overflow: hidden;

    /* Separates the paper from the card when the two are close in tone. */
    border-bottom: 1px solid color-mix(in srgb, var(--jp-color-outline) 30%, transparent);
    /* The paper color is set inline from the document. */
    color: var(--ori-color-on-surface);
}

.drawing__img,
.drawing__skeleton {
    position: absolute;
    inset: 0;

    width: 100%;
    height: 100%;
}

/* The image is cut at the preview's own aspect, so cover never crops. */
.drawing__img {
    object-fit: cover;
}

.drawing__missing {
    opacity: 0.5;
}

.drawing__meta {
    display: flex;
    flex-direction: column;

    padding: var(--ori-size-gap_lg) var(--drawing-inset-x) var(--drawing-inset-bottom);
}

.drawing__name {
    overflow: hidden;

    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
}

/* The footer row is as tall as the menu button and leaves room for it: the button floats
   over the card's bottom-right corner. */
.drawing__time {
    display: flex;
    align-items: center;

    min-height: var(--ori-size-action_md);
    padding-inline-end: calc(var(--ori-size-action_md) + var(--ori-size-gap_sm));

    font-size: var(--ori-font-size_sm);
    /* 0.7 keeps the muted line past WCAG AA on the surface. */
    opacity: 0.7;
}

/* Sits on the footer row, at the meta block's corner (its insets plus the card's 1px border). */
.drawing__actions {
    position: absolute;
    right: calc(var(--drawing-inset-x) + 1px);
    bottom: calc(var(--drawing-inset-bottom) + 1px);
}

.drawing__trigger {
    display: flex;
    transition: transform 160ms ease;
}

.drawing:hover .drawing__trigger,
.drawing:focus-within .drawing__trigger {
    transform: translateY(-2px);
}

.drawing__item {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_md);
}

.drawing__item--danger {
    color: var(--ori-color-danger-text, var(--ori-color-danger));
}

/* Narrow cards need the room for the time. */
@media (width <= 600px) {
    .drawing {
        --drawing-inset-x: var(--ori-size-gap_lg);
    }
}

@media (prefers-reduced-motion: reduce) {
    .drawing__lift,
    .drawing__trigger {
        transition: none;
    }

    .drawing:hover .drawing__lift,
    .drawing:focus-within .drawing__lift,
    .drawing:hover .drawing__trigger,
    .drawing:focus-within .drawing__trigger {
        transform: none;
    }
}
</style>
