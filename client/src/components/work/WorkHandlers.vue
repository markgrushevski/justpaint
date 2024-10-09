<script setup lang="ts">
import { copyToClipboard, getCanvasBlob } from '@shared/lib'
import { useCanvasHistoryStore, useCanvasStore } from '@shared/stores'
import { WorkIcon } from '@shared/ui'
import { ref } from 'vue'

const historyStore = useCanvasHistoryStore()

const icons = ref([
    {
        name: 'undo',
        isSuccess: false,
        isError: false
    },
    {
        name: 'redo',
        isSuccess: false,
        isError: false
    },
    {
        name: 'save',
        isSuccess: false,
        isError: false
    },
    {
        name: 'load',
        isSuccess: false,
        isError: false
    },
    {
        name: 'copy',
        isSuccess: false,
        isError: false
    }
])

async function actionHandler(ev: UIEvent, actionName: (typeof icons.value)[number]['name']) {
    const icon = icons.value.find((icon) => icon.name === actionName)
    if (!icon) return

    if (actionName === 'copy') {
        const canvas = useCanvasStore().canvas

        if (canvas) {
            const canvasBlob = await getCanvasBlob(canvas)
            await copyToClipboard('image', canvasBlob)
            handleSuccess(icon)
        } else {
            console.error('Канвас не найден')
            handleError(icon)
        }
    } else {
        const historyHandler = historyStore.historyHandler?.eventHandlersMap?.[actionName]
        if (historyHandler) {
            historyHandler(ev)
        } else {
            console.error('Не найден обработчик')
            handleError(icon)
        }
    }
}

function handleSuccess(icon: (typeof icons.value)[number]) {
    icon.isSuccess = true
    setTimeout(() => {
        icon.isSuccess = false
    }, 300)
}

function handleError(icon: (typeof icons.value)[number]) {
    icon.isError = true
    setTimeout(() => {
        icon.isError = false
    }, 300)
}
</script>

<template>
    <WorkIcon
        v-for="{ name, isSuccess, isError } in icons"
        :key="name"
        :icon-name="name as any"
        :is-success="isSuccess"
        :is-error="isError"
        :title="name.charAt(0).toUpperCase() + name.substring(1)"
        width="1.75rem"
        height="1.75rem"
        @click="actionHandler($event, name)"
    />
</template>
