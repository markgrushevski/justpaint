import axios from 'axios'
import type { UserLogin, Work } from './types.ts'

const API = axios.create({
    baseURL: import.meta.env.VITE_URL_API
})

API.interceptors.request.use((req) => {
    const token = localStorage.getItem('token')
    req.headers['Authorization'] = `Bearer ${token}`
    return req
})

export const mainAPI = {
    auth: {
        login: async (userLogin: UserLogin) => {
            return API.post('/login', userLogin).then((response) => response?.data)
        },
        logout: async () => {
            return API.post('/logout').then((response) => response?.data)
        }
    },
    works: {
        getWorks: async (): Promise<Work[]> => {
            console.log('getWorks')
            return API.get('/works').then((response) => response?.data)
        },
        saveWork: async (work: Work) => {
            return API.post('/works', work).then((response) => response?.data)
        }
    }
}
