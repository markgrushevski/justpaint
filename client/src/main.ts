import { router, pinia } from '@app'
import { createApp } from 'vue'
import App from './TheApp.vue'

const justpaintApp = createApp(App).use(router).use(pinia)

justpaintApp.mount('#justpaint')
