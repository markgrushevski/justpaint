import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
    { path: '/', redirect: '/draw' },
    { path: '/draw', name: 'draw', component: () => import('../views/DrawView.vue') },
    { path: '/play', name: 'play', component: () => import('../views/PlayView.vue') },
    { path: '/leaderboard', name: 'leaderboard', component: () => import('../views/LeaderboardView.vue') }
]

export const router = createRouter({
    history: createWebHistory(),
    routes
})
