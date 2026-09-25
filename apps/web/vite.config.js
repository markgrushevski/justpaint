import vue from '@vitejs/plugin-vue'
import autoprefixer from 'autoprefixer'
import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'

export default defineConfig({
    plugins: [vue()],
    server: {
        port: 7777,
        // Dev: forward same-origin /api to the Go server so the jp_session cookie
        // is first-party (no CORS); in production the Go binary serves the SPA, so
        // there is no proxy. `ws: true` also forwards the WebSocket upgrade for
        // GET /api/matches/:id/ws — without it the dev proxy drops the upgrade.
        proxy: { '/api': { target: 'http://localhost:8080', changeOrigin: true, ws: true } }
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
