import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { VueQueryPlugin } from '@tanstack/vue-query'
import App from './TheApp.vue'

createApp(App).use(VueQueryPlugin).use(createPinia()).mount('#app')
