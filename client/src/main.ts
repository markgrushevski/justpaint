import { router, pinia } from '@app';
import { createApp } from 'vue';
import App from './TheApp.vue';

createApp(App).use(router).use(pinia).mount('#app');
