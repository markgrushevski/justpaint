import '@oriui/css'
import './reset.css'
import './main.css'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { router } from './router'
import App from './App.vue'

createApp(App).use(router).use(VueQueryPlugin).use(createPinia()).mount('#app')
