// oriUI à-la-carte CSS: the foundation entry MUST come before any component css
// (it declares the @layer order + reset + tokens + utilities the components rely on).
import '@oriui/css/base.css'
// One file per Ori* component used, alphabetical. Each file is self-contained:
// it inlines its transitive deps (button.css already carries icon + spinner css).
import '@oriui/css/components/avatar.css'
import '@oriui/css/components/badge.css'
import '@oriui/css/components/button.css'
import '@oriui/css/components/card.css'
import '@oriui/css/components/checkbox.css'
import '@oriui/css/components/dialog.css'
import '@oriui/css/components/field.css'
import '@oriui/css/components/icon.css'
import '@oriui/css/components/input.css'
import '@oriui/css/components/join.css'
import '@oriui/css/components/kbd.css'
import '@oriui/css/components/popover.css'
import '@oriui/css/components/select.css'
import '@oriui/css/components/skeleton.css'
import '@oriui/css/components/slider.css'
import '@oriui/css/components/surface.css'
import '@oriui/css/components/switch.css'
import '@oriui/css/components/tabs.css'
import '@oriui/css/components/toast.css'
import '@oriui/css/components/toolbar.css'
import '@oriui/css/components/tooltip.css'
import './reset.css'
import './main.css'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { router } from './router'
import { setUnauthorizedHandler, useAuthGate, useSessionStore } from './core'
import App from './App.vue'

const pinia = createPinia()
const app = createApp(App).use(router).use(VueQueryPlugin).use(pinia)

// The composition root, where the store-free fetch client meets the store: any
// 401 forgets the session, everywhere, once. Asking for a NEW one stays a UI
// decision (docs/DECISIONS.md) — a background poll must not raise a modal.
setUnauthorizedHandler(() => useSessionStore(pinia).clear())

// The sign-in modal is global, so a route change has to release it — otherwise it
// follows the visitor to the next page still holding the previous page's waiter.
router.afterEach(() => useAuthGate(pinia).settle(false))

app.mount('#app')
