<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import { useAuth } from '@/stores/auth'
import { http } from '@/api/http'
import { useNotify } from '@/composables/useApiAction'
import { tagFromScan } from '@/utils/format'

const auth = useAuth()
const route = useRoute()
const router = useRouter()
const notify = useNotify()

const menu = [
  { title: 'Menu', items: [{ to: 'dashboard', label: 'Dashboard', icon: 'pi-home' }] },
  {
    title: 'Inventaris',
    items: [
      { to: 'asset', label: 'Daftar Aset', icon: 'pi-box' },
      { to: 'scan', label: 'Scan Kode', icon: 'pi-qrcode' },
      { to: 'labels', label: 'Cetak Label', icon: 'pi-print' },
    ],
  },
  {
    title: 'Serah Terima',
    items: [
      { to: 'handover.create', label: 'Buat Dokumen', icon: 'pi-arrow-right-arrow-left' },
      { to: 'handover', label: 'Riwayat Dokumen', icon: 'pi-list' },
    ],
  },
  {
    title: 'Master Data',
    items: [
      { to: 'employee', label: 'Karyawan', icon: 'pi-users' },
      { to: 'category', label: 'Kategori', icon: 'pi-tags' },
    ],
  },
  {
    title: 'Laporan',
    items: [
      { to: 'monitor.employee', label: 'Inventaris Karyawan', icon: 'pi-user' },
      { to: 'activity', label: 'Log Aktivitas', icon: 'pi-history' },
    ],
  },
]

const open = ref(false)
watch(() => route.fullPath, () => (open.value = false))

const dark = ref(false)
try {
  dark.value = localStorage.getItem('theme') === 'dark'
} catch {
  /* storage unavailable */
}
function applyTheme() {
  document.documentElement.classList.toggle('app-dark', dark.value)
}
applyTheme()
function toggleTheme() {
  dark.value = !dark.value
  applyTheme()
  try {
    localStorage.setItem('theme', dark.value ? 'dark' : 'light')
  } catch {
    /* ignore */
  }
}

const title = computed(() => (route.meta.title as string) ?? '')

// Quick jump: type or scan (USB scanner) a tag anywhere and press Enter.
const quick = ref('')
function jump() {
  const raw = quick.value.trim()
  if (!raw) return
  quick.value = ''
  const tag = tagFromScan(raw)
  if (/^[A-Z]{2,5}-\d+$/.test(tag)) router.push({ name: 'asset.detail', params: { tag } })
  else router.push({ name: 'asset', query: { search: raw } })
}

async function logout() {
  await auth.logout()
  router.push({ name: 'login' })
}

const pwVisible = ref(false)
const pw = reactive({ current_password: '', new_password: '' })
const pwErrors = ref<Record<string, string>>({})
const pwBusy = ref(false)
function openPassword() {
  Object.assign(pw, { current_password: '', new_password: '' })
  pwErrors.value = {}
  pwVisible.value = true
}
async function savePassword() {
  pwBusy.value = true
  pwErrors.value = {}
  try {
    await http.put('/auth/password', pw)
    notify.success('Password berhasil diubah')
    pwVisible.value = false
  } catch (e) {
    pwErrors.value = notify.fromError(e)
  } finally {
    pwBusy.value = false
  }
}
</script>

<template>
  <div class="shell">
    <div v-if="open" class="scrim no-print" @click="open = false" />
    <aside class="sidebar no-print" :class="{ open }">
      <div class="brand">
        <i class="pi pi-desktop" />
        <span>Aset IT</span>
      </div>
      <nav>
        <template v-for="group in menu" :key="group.title">
          <div class="group-title">{{ group.title }}</div>
          <RouterLink v-for="item in group.items" :key="item.to" :to="{ name: item.to }" class="nav-link" active-class="active">
            <i class="pi" :class="item.icon" />
            <span>{{ item.label }}</span>
          </RouterLink>
        </template>
      </nav>
    </aside>

    <div class="main">
      <header class="topbar no-print">
        <Button class="burger" icon="pi pi-bars" text rounded aria-label="Menu" @click="open = !open" />
        <h1>{{ title }}</h1>
        <form class="quick" @submit.prevent="jump">
          <i class="pi pi-search" />
          <InputText v-model="quick" placeholder="Scan / ketik kode aset…" size="small" aria-label="Cari kode aset" />
        </form>
        <div class="spacer" />
        <Button :icon="dark ? 'pi pi-sun' : 'pi pi-moon'" text rounded aria-label="Ganti tema" @click="toggleTheme" />
        <Button class="user" icon="pi pi-user" :label="auth.user?.name" text size="small" severity="secondary" aria-label="Ganti password" @click="openPassword" />
        <Button label="Logout" icon="pi pi-sign-out" severity="secondary" text size="small" @click="logout" />
      </header>
      <main class="content">
        <RouterView />
      </main>
    </div>
  </div>

  <Dialog v-model:visible="pwVisible" modal header="Ganti Password" :style="{ width: '26rem', maxWidth: '95vw' }">
    <form novalidate @submit.prevent="savePassword">
      <p class="muted" style="margin-top: 0">Masuk sebagai {{ auth.user?.email }}</p>
      <div class="field">
        <label for="pw-cur">Password saat ini</label>
        <Password v-model="pw.current_password" input-id="pw-cur" :feedback="false" toggle-mask autocomplete="current-password" :invalid="!!pwErrors.current_password" fluid />
        <small v-if="pwErrors.current_password" class="field-error">{{ pwErrors.current_password }}</small>
      </div>
      <div class="field">
        <label for="pw-new">Password baru (min. 8 karakter)</label>
        <Password v-model="pw.new_password" input-id="pw-new" :feedback="false" toggle-mask autocomplete="new-password" :invalid="!!pwErrors.new_password" fluid />
        <small v-if="pwErrors.new_password" class="field-error">{{ pwErrors.new_password }}</small>
      </div>
      <div class="toolbar" style="justify-content: flex-end">
        <Button type="button" label="Batal" severity="secondary" text @click="pwVisible = false" />
        <Button type="submit" label="Simpan" icon="pi pi-check" :loading="pwBusy" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.shell { display: flex; min-height: 100%; }
.sidebar {
  width: var(--sidebar-w); flex: none; background: var(--p-content-background);
  border-right: 1px solid var(--p-content-border-color); padding: 1rem 0.75rem;
  position: sticky; top: 0; height: 100vh; overflow-y: auto;
}
.brand { display: flex; gap: 0.6rem; align-items: center; font-weight: 700; padding: 0.25rem 0.75rem 1rem; font-size: 1.05rem; }
.brand .pi { color: var(--p-primary-color); font-size: 1.3rem; }
.group-title { font-size: 0.7rem; text-transform: uppercase; letter-spacing: 0.06em; color: var(--p-text-muted-color); padding: 1rem 0.75rem 0.35rem; }
.nav-link {
  display: flex; gap: 0.7rem; align-items: center; padding: 0.55rem 0.75rem; border-radius: 0.5rem;
  color: var(--p-text-color); text-decoration: none; font-size: 0.92rem;
}
.nav-link:hover { background: var(--p-surface-100); }
.app-dark .nav-link:hover { background: var(--p-surface-800); }
.nav-link.active { background: var(--p-primary-50); color: var(--p-primary-700); font-weight: 600; }
.app-dark .nav-link.active { background: color-mix(in srgb, var(--p-primary-color) 18%, transparent); color: var(--p-primary-300); }
.main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
.topbar {
  display: flex; align-items: center; gap: 0.5rem; padding: 0.6rem 1.25rem; background: var(--p-content-background);
  border-bottom: 1px solid var(--p-content-border-color); position: sticky; top: 0; z-index: 20;
}
.topbar h1 { font-size: 1.05rem; margin: 0 0.5rem 0 0.25rem; white-space: nowrap; }
.quick { position: relative; display: flex; align-items: center; }
.quick .pi { position: absolute; left: 0.6rem; color: var(--p-text-muted-color); font-size: 0.85rem; pointer-events: none; }
.quick :deep(input) { padding-left: 2rem; width: 15rem; max-width: 40vw; }
.spacer { flex: 1; }
.content { padding: 1.25rem; }
.burger { display: none; }
.scrim { display: none; }
@media (max-width: 900px) {
  .sidebar { position: fixed; left: 0; top: 0; z-index: 40; transform: translateX(-100%); transition: transform 0.2s; }
  .sidebar.open { transform: none; box-shadow: 0 0 40px rgb(0 0 0 / 0.25); }
  .scrim { display: block; position: fixed; inset: 0; background: rgb(0 0 0 / 0.4); z-index: 30; }
  .burger { display: inline-flex; }
  .topbar h1 { display: none; }
  .user :deep(.p-button-label) { display: none; }
  .content { padding: 0.75rem; }
}
</style>
