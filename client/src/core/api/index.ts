function appFetch(path: string, config: RequestInit = {}) {
    const base = import.meta.env.VITE_URL_API
    const url = new URL(path, base)
    config.headers = {
        ...config.headers
    }
    return fetch(url, config)
}

export const API = {
    getHome: async () => {
        return appFetch('/', { method: 'GET' }).then((res) => res.text())
    },
    getWorks: async () => {
        return appFetch('/works', { method: 'GET' }).then((res) => res.json())
    },
    saveWork: async () => {
        return appFetch('/works', {
            method: 'POST',
            headers: { 'content-type': 'application/json' },
            body: JSON.stringify([{ value: 'new work ' + Date.now() }])
        }).then((res) => res.json())
    }
}
