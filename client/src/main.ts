import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './TheApp.vue'

createApp(App).use(createPinia()).mount('#app')
