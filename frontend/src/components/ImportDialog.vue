<script setup lang="ts">
import { ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Message from 'primevue/message'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import { ApiError } from '@/api/http'
import { useNotify } from '@/composables/useApiAction'

interface Result {
  dry_run: boolean
  rows: number
  created: number
  categories_created: string[]
  employees_created: string[]
  errors: { row: number; message: string }[]
}

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [boolean]; imported: [] }>()
const notify = useNotify()

const file = ref<File | null>(null)
const result = ref<Result | null>(null)
const failure = ref('')
const busy = ref(false)

watch(() => props.visible, (v) => {
  if (v) {
    file.value = null
    result.value = null
    failure.value = ''
  }
})

function pick(e: Event) {
  const input = e.target as HTMLInputElement
  file.value = input.files?.[0] ?? null
  result.value = null
  failure.value = ''
}

// fetch (not the shared client): a 422 here still carries a full report we want to show
async function send(dry: boolean) {
  if (!file.value) return
  busy.value = true
  failure.value = ''
  try {
    const fd = new FormData()
    fd.append('file', file.value)
    const res = await fetch('/api/assets/import' + (dry ? '?dry_run=1' : ''), { method: 'POST', body: fd, credentials: 'same-origin' })
    const body = await res.json()
    if (res.ok || body?.data?.errors) {
      result.value = body.data as Result
      if (res.ok && !dry) {
        notify.success(`${body.data.created} aset berhasil diimpor`)
        emit('imported')
      }
    } else {
      failure.value = body?.message ?? 'Impor gagal'
    }
  } catch (e) {
    failure.value = e instanceof ApiError ? e.message : 'Tidak dapat terhubung ke server'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog :visible="visible" modal header="Impor Aset dari CSV" :style="{ width: '44rem', maxWidth: '96vw' }" @update:visible="emit('update:visible', $event)">
    <ol class="steps">
      <li>Unduh <a class="link" href="/api/assets/import/template" download>template CSV</a> lalu isi dari Excel/Google Sheets (Simpan sebagai CSV). Hanya kolom <b>Kategori</b> yang wajib.</li>
      <li>Pilih file, klik <b>Periksa dulu</b> untuk melihat hasilnya tanpa menyimpan.</li>
      <li>Jika sudah benar, klik <b>Impor</b>. Jika ada satu baris bermasalah, tidak ada data yang disimpan.</li>
    </ol>
    <p class="muted hint">
      Status: Tersedia, Dipakai, Diservis, Rusak, Dimusnahkan. Isi kolom <b>Pengguna</b> untuk aset yang dipakai karyawan (karyawan baru dibuat otomatis).
      Tanggal boleh <span class="mono">2025-10-10</span>, <span class="mono">10/10/2025</span>, atau <span class="mono">10 Oktober 2025</span>.
    </p>

    <input type="file" accept=".csv,text/csv" class="file" @change="pick" />

    <Message v-if="failure" severity="error" :closable="false" class="gap">{{ failure }}</Message>

    <template v-if="result">
      <Message v-if="result.errors.length" severity="error" :closable="false" class="gap">
        {{ result.errors.length }} masalah ditemukan. Tidak ada data yang disimpan. Perbaiki file lalu coba lagi.
      </Message>
      <Message v-else :severity="result.dry_run ? 'info' : 'success'" :closable="false" class="gap">
        {{ result.dry_run ? 'Siap diimpor' : 'Selesai' }}: {{ result.created }} dari {{ result.rows }} baris menjadi aset.
        <span v-if="result.categories_created.length"> Kategori baru: {{ result.categories_created.join(', ') }}.</span>
        <span v-if="result.employees_created.length"> Karyawan baru: {{ result.employees_created.join(', ') }}.</span>
      </Message>
      <DataTable v-if="result.errors.length" :value="result.errors" size="small" scrollable scroll-height="14rem">
        <Column field="row" header="Baris" style="width: 5rem" />
        <Column field="message" header="Masalah" />
      </DataTable>
    </template>

    <div class="toolbar actions">
      <Button label="Tutup" severity="secondary" text @click="emit('update:visible', false)" />
      <Button label="Periksa dulu" icon="pi pi-search" severity="secondary" :disabled="!file" :loading="busy" @click="send(true)" />
      <Button label="Impor" icon="pi pi-upload" :disabled="!file || (result?.errors.length ?? 0) > 0 || !!(result && !result.dry_run)" :loading="busy" @click="send(false)" />
    </div>
  </Dialog>
</template>

<style scoped>
.steps { margin: 0 0 0.75rem; padding-left: 1.2rem; display: grid; gap: 0.35rem; }
.hint { font-size: 0.85rem; margin: 0 0 1rem; }
.file { margin-bottom: 1rem; max-width: 100%; }
.gap { margin: 0.75rem 0; }
.actions { justify-content: flex-end; margin-top: 1rem; }
</style>
