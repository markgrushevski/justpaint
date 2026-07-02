import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
    { path: '/', redirect: '/draw' },
    { path: '/draw', name: 'draw', component: () => import('../views/DrawView.vue') }
]

export const router = createRouter({
    history: createWebHistory(),
    routes
})
