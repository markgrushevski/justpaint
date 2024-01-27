<script setup lang="ts">
defineProps<{
    isActive?: boolean;
    type: 'undo' | 'redo' | 'save';
}>();
</script>

<template>
    <div class="work-handlers__handler" :class="{ 'work-handlers__handler_active': isActive }">
        <svg v-if="type === 'undo'" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
            <path
                d="M12.5,8C9.85,8 7.45,9 5.6,10.6L2,7V16H11L7.38,12.38C8.77,11.22 10.54,10.5 12.5,10.5C16.04,10.5 19.05,12.81 20.1,16L22.47,15.22C21.08,11.03 17.15,8 12.5,8Z"
            />
        </svg>

        <svg v-else-if="type === 'redo'" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
            <path
                d="M18.4,10.6C16.55,9 14.15,8 11.5,8C6.85,8 2.92,11.03 1.54,15.22L3.9,16C4.95,12.81 7.95,10.5 11.5,10.5C13.45,10.5 15.23,11.22 16.62,12.38L13,16H22V7L18.4,10.6Z"
            />
        </svg>

        <svg v-else-if="type === 'save'" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
            <path
                d="M17 3H5C3.89 3 3 3.9 3 5V19C3 20.1 3.89 21 5 21H19C20.1 21 21 20.1 21 19V7L17 3M19 19H5V5H16.17L19 7.83V19M12 12C10.34 12 9 13.34 9 15S10.34 18 12 18 15 16.66 15 15 13.66 12 12 12M6 6H15V10H6V6Z"
            />
        </svg>
    </div>
</template>

<style>
.work-handlers__handler {
    width: 1.75em;
    height: 1.75em;
}

.work-handlers__handler > * {
    width: 100%;
    height: 100%;
    fill: var(--color-tool);
    cursor: pointer;
}

.work-handlers__handler_active > *,
.work-handlers__handler:hover > * {
    fill: var(--color-text);
}
</style>
