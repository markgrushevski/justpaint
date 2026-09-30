import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
    { path: '/', redirect: '/draw' },
    { path: '/draw', name: 'draw', component: () => import('../features/draw/DrawView.vue') },
    { path: '/play', name: 'play', component: () => import('../features/play/PlayView.vue') },
    { path: '/practice', name: 'practice', component: () => import('../features/practice/PracticeView.vue') },
    {
        path: '/leaderboard',
        name: 'leaderboard',
        component: () => import('../features/leaderboard/LeaderboardView.vue')
    }
]

export const router = createRouter({
    history: createWebHistory(),
    routes
})
