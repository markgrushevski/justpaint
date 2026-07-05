// oriUI à-la-carte CSS: the foundation entry MUST come before any component css
// (it declares the @layer order + reset + tokens + utilities the components rely on).
import '@oriui/css/base.css'
// One file per Ori* component used, alphabetical. icon + spinner are transitive:
// OriButton renders OriIcon and, when loading, OriSpinner.
import '@oriui/css/components/avatar.css'
import '@oriui/css/components/button.css'
import '@oriui/css/components/checkbox.css'
import '@oriui/css/components/field.css'
import '@oriui/css/components/icon.css'
import '@oriui/css/components/input.css'
import '@oriui/css/components/kbd.css'
import '@oriui/css/components/slider.css'
import '@oriui/css/components/spinner.css'
import '@oriui/css/components/tabs.css'
import './reset.css'
import './main.css'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { router } from './router'
import App from './App.vue'

createApp(App).use(router).use(VueQueryPlugin).use(createPinia()).mount('#app')
