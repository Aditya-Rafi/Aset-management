import { createApp } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import Tooltip from 'primevue/tooltip'
import Aura from '@primeuix/themes/aura'
import 'primeicons/primeicons.css'
import './style.css'

import App from './App.vue'
import router from './router'
import { setUnauthorizedHandler } from './api/http'
import { useAuth } from './stores/auth'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)
app.use(PrimeVue, {
  theme: { preset: Aura, options: { darkModeSelector: '.app-dark', cssLayer: false } },
  locale: {
    emptyMessage: 'Tidak ada data',
    emptyFilterMessage: 'Tidak ada hasil',
    dayNames: ['Minggu', 'Senin', 'Selasa', 'Rabu', 'Kamis', 'Jumat', 'Sabtu'],
    monthNames: ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni', 'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember'],
  },
})
app.use(ToastService)
app.use(ConfirmationService)
app.directive('tooltip', Tooltip)

// Any 401 from the API means the session ended: drop the user and go to login.
setUnauthorizedHandler(() => {
  const auth = useAuth()
  if (auth.user) {
    auth.user = null
    router.push({ name: 'login' })
  }
})

app.mount('#app')
