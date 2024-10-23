import type { UserLogin, Work } from './types.ts'

async function appFetch<T>(path: string, config: RequestInit = {}) {
    const base = import.meta.env.VITE_URL_API
    const url = new URL(path, base)
    const token = localStorage.getItem('token')
    if (token) {
        config.headers = {
            ...config.headers,
            Authorization: `Bearer ${token}`
        }
    }
    return fetch(url, config).then<T>((res) => res.json())
}

export const API = {
    home: async () => {
        return appFetch('', {
            method: 'GET'
        })
    },
    auth: {
        login: async (userLogin: UserLogin) => {
            return appFetch('/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(userLogin)
            })
        },
        logout: async () => {
            return appFetch('/login', {
                method: 'POST'
            })
        }
    },
    works: {
        getWorks: async (): Promise<Work[]> => {
            return appFetch('/works', { method: 'GET' })
        },
        saveWork: async (work: Work) => {
            return appFetch('/works', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(work)
            })
        }
    }
}
