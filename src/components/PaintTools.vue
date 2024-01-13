<script setup lang="ts">
import { PaintTool } from '@components';
</script>

<template>
    <div class="paint-tools">
        <PaintTool>
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                <title>Pen</title>
                <path
                    d="M20.71,7.04C20.37,7.38 20.04,7.71 20.03,8.04C20,8.36 20.34,8.69 20.66,9C21.14,9.5 21.61,9.95 21.59,10.44C21.57,10.93 21.06,11.44 20.55,11.94L16.42,16.08L15,14.66L19.25,10.42L18.29,9.46L16.87,10.87L13.12,7.12L16.96,3.29C17.35,2.9 18,2.9 18.37,3.29L20.71,5.63C21.1,6 21.1,6.65 20.71,7.04M3,17.25L12.56,7.68L16.31,11.43L6.75,21H3V17.25Z"
                />
            </svg>
        </PaintTool>
        <PaintTool>
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
                <title>Eraser</title>
                <path
                    d="M16.24,3.56L21.19,8.5C21.97,9.29 21.97,10.55 21.19,11.34L12,20.53C10.44,22.09 7.91,22.09 6.34,20.53L2.81,17C2.03,16.21 2.03,14.95 2.81,14.16L13.41,3.56C14.2,2.78 15.46,2.78 16.24,3.56M4.22,15.58L7.76,19.11C8.54,19.9 9.8,19.9 10.59,19.11L14.12,15.58L9.17,10.63L4.22,15.58Z"
                />
            </svg>
        </PaintTool>
        <PaintTool>
            <input type="color" />
        </PaintTool>
    </div>
</template>

<style>
.paint-tools {
    display: flex;
    align-items: center;
    gap: 4px;
}
</style>
