<script setup lang="ts">
import { useDateFormat, useNow } from '@vueuse/core'
import { WorkIcon } from '@shared/ui'

const formattedDate = useDateFormat(useNow(), 'HH:mm')
</script>

<template>
    <WorkIcon class="system-bar__menu-icon" icon-path-value="M3,6H21V8H3V6M3,11H21V13H3V11M3,16H21V18H3V16Z" />
    <div class="system-bar__work-title">My Untitled Work</div>
    <div class="system-bar__weather">0</div>
    <div class="system-bar__time">{{ formattedDate }}</div>
</template>

<style>
.system-bar__menu-icon {
    margin-right: auto;
}
</style>
