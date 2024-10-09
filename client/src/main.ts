import { createApp } from 'vue'
import { router, pinia } from '@app'
import App from './TheApp.vue'

const justpaintApp = createApp(App).use(router).use(pinia)

justpaintApp.mount('#justpaint')
