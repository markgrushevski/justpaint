export function appFetch(path: string, config?: RequestInit) {
    const base = import.meta.env.VITE_URL_API;
    const url = new URL(path, base);
    return fetch(url, config);
}
