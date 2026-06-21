import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
    { path: '/', redirect: '/draw' },
    { path: '/draw', name: 'draw', component: () => import('../views/DrawView.vue') },
    // The legacy raster paint app, parked off the default path for reference.
    { path: '/legacy', name: 'legacy', component: () => import('../TheApp.vue') }
]

export const router = createRouter({
    history: createWebHistory(),
    routes
})
