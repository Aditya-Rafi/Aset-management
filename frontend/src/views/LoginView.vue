<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useAuth } from '@/stores/auth'
import { ApiError } from '@/api/http'
import { useNotify } from '@/composables/useApiAction'

const auth = useAuth()
const router = useRouter()
const route = useRoute()

// only follow redirects inside this app (never to another site)
const safeRedirect = (r: unknown) => (typeof r === 'string' && r.startsWith('/') && !r.startsWith('//') ? r : { name: 'dashboard' })
const notify = useNotify()

const form = reactive({ email: '', password: '' })
const errors = ref<Record<string, string>>({})
const failure = ref('')
const busy = ref(false)

async function submit() {
  errors.value = {}
  failure.value = ''
  busy.value = true
  try {
    const msg = await auth.login(form.email, form.password)
    notify.success(msg ?? 'Login berhasil')
    router.push(safeRedirect(route.query.redirect))
  } catch (e) {
    if (e instanceof ApiError) {
      errors.value = e.errors
      if (!Object.keys(e.errors).length) failure.value = e.message
    } else failure.value = 'Terjadi kesalahan tak terduga'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login">
    <form class="card" @submit.prevent="submit" novalidate>
      <div class="logo"><i class="pi pi-box" /></div>
      <h1>Aset IT</h1>
      <p class="muted">Silakan login untuk melanjutkan</p>

      <Message v-if="failure" severity="error" :closable="false" class="msg">{{ failure }}</Message>

      <div class="field">
        <label for="email">Email</label>
        <InputText id="email" v-model="form.email" type="email" autocomplete="username" autofocus :invalid="!!errors.email" fluid />
        <small v-if="errors.email" class="field-error">{{ errors.email }}</small>
      </div>
      <div class="field">
        <label for="password">Password</label>
        <Password v-model="form.password" input-id="password" :feedback="false" toggle-mask autocomplete="current-password" :invalid="!!errors.password" fluid />
        <small v-if="errors.password" class="field-error">{{ errors.password }}</small>
      </div>
      <Button type="submit" label="Login" icon="pi pi-sign-in" :loading="busy" fluid />
    </form>
  </div>
</template>

<style scoped>
.login { min-height: 100%; display: grid; place-items: center; padding: 1.5rem; background: linear-gradient(135deg, var(--p-primary-50), var(--p-surface-100)); }
.app-dark .login { background: var(--p-surface-950); }
.card { width: 100%; max-width: 24rem; background: var(--p-content-background); border: 1px solid var(--p-content-border-color); border-radius: 1rem; padding: 2rem; box-shadow: 0 10px 40px rgb(0 0 0 / 0.08); }
.logo { width: 3rem; height: 3rem; border-radius: 0.8rem; background: var(--p-primary-color); color: #fff; display: grid; place-items: center; font-size: 1.4rem; margin-bottom: 1rem; }
h1 { margin: 0; font-size: 1.3rem; }
p { margin: 0.25rem 0 1.5rem; }
.msg { margin-bottom: 1rem; }
</style>
