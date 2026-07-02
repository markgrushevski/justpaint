import vue from '@vitejs/plugin-vue'
import autoprefixer from 'autoprefixer'
import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'

export default defineConfig({
    plugins: [vue()],
    server: {
        port: 7777,
        // Dev: forward same-origin /api to the Go server so the jp_session cookie
        // is first-party (no CORS). Production uses a real reverse proxy.
        proxy: { '/api': { target: 'http://localhost:8080', changeOrigin: true } }
    },
    preview: { port: 7777 },
    build: { sourcemap: true },
    css: { postcss: { plugins: [autoprefixer] }, devSourcemap: true },
    resolve: {
        alias: {
            '@core': fileURLToPath(new URL('./src/core', import.meta.url))
        }
    }
})
