import vue from '@vitejs/plugin-vue'
import autoprefixer from 'autoprefixer'
import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'

export default defineConfig({
    plugins: [vue()],
    server: { port: 7777 },
    preview: { port: 7777 },
    build: { sourcemap: true },
    esbuild: { sourcemap: 'external' },
    css: { postcss: { plugins: [autoprefixer] }, devSourcemap: true },
    resolve: {
        alias: {
            '@core': fileURLToPath(new URL('./src/core', import.meta.url)),
            '@modules': fileURLToPath(new URL('./src/modules', import.meta.url))
        }
    }
})
