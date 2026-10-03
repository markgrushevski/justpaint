import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

/** The mode a route belongs to; `ModeNav` marks it. The gallery is part of drawing. */
export type AppMode = 'draw' | 'practice' | 'duel'

declare module 'vue-router' {
    interface RouteMeta {
        mode?: AppMode
    }
}

const routes: RouteRecordRaw[] = [
    { path: '/', redirect: '/draw' },
    { path: '/draw', name: 'draw', component: () => import('../features/draw/DrawView.vue'), meta: { mode: 'draw' } },
    {
        path: '/gallery',
        name: 'gallery',
        component: () => import('../features/gallery/GalleryView.vue'),
        meta: { mode: 'draw' }
    },
    { path: '/play', name: 'play', component: () => import('../features/play/PlayView.vue'), meta: { mode: 'duel' } },
    {
        path: '/practice',
        name: 'practice',
        component: () => import('../features/practice/PracticeView.vue'),
        meta: { mode: 'practice' }
    },
    {
        path: '/leaderboard',
        name: 'leaderboard',
        component: () => import('../features/leaderboard/LeaderboardView.vue'),
        meta: { mode: 'duel' }
    }
]

export const router = createRouter({
    history: createWebHistory(),
    routes
})
