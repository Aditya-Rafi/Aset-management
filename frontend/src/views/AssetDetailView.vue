<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'
import Message from 'primevue/message'
import { useConfirm } from 'primevue/useconfirm'
import { http, ApiError, type Envelope } from '@/api/http'
import { useNotify } from '@/composables/useApiAction'
import { useLookups } from '@/stores/lookups'
import StatusTag from '@/components/StatusTag.vue'
import AssetForm from '@/components/AssetForm.vue'
import Tag from 'primevue/tag'
import {
  CONCLUSION, LOG_ACTIONS, txTypeLabel, SPEC_LABELS, assetTitle, dash, daysUntil, formatDate, formatDateTime, formatRupiah, statusLabel, statusOptions,
} from '@/utils/format'
import type { Asset, AssetLog, ConditionReport } from '@/types/models'

const route = useRoute()
const router = useRouter()
const notify = useNotify()
const confirm = useConfirm()
const lookups = useLookups()

const asset = ref<Asset | null>(null)
const logs = ref<AssetLog[]>([])
const report = ref<ConditionReport | null>(null)
const loading = ref(true)
const notFound = ref(false)
const error = ref('')

const tag = computed(() => String(route.params.tag).toUpperCase())

async function loadLogs() {
  if (!asset.value) return
  logs.value = (await http.get<Envelope<AssetLog[]>>(`/assets/${asset.value.id}/logs`)).data
}

async function loadReport() {
  if (!asset.value) return
  report.value = (await http.get<Envelope<ConditionReport>>(`/assets/${asset.value.id}/condition-report`)).data
}

async function load() {
  loading.value = true
  notFound.value = false
  error.value = ''
  asset.value = null
  try {
    asset.value = (await http.get<Envelope<Asset>>(`/assets/tag/${encodeURIComponent(tag.value)}`)).data
    await Promise.all([loadLogs(), loadReport()])
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) notFound.value = true
    else error.value = e instanceof Error ? e.message : 'Gagal memuat aset'
  } finally {
    loading.value = false
  }
}
watch(tag, load, { immediate: true })
lookups.loadConfig().catch(() => {})

const specs = computed(() => {
  const a = asset.value
  if (!a) return []
  const wanted = a.category?.spec_fields ?? []
  // show enabled fields, plus any field that already has a value
  return Object.keys(SPEC_LABELS)
    .filter((k) => wanted.includes(k as never) || (a as unknown as Record<string, unknown>)[k])
    .map((k) => ({ label: SPEC_LABELS[k], value: (a as unknown as Record<string, string | null>)[k] }))
})

const warranty = computed(() => {
  const n = daysUntil(asset.value?.warranty_until)
  if (n === null) return null
  if (n < 0) return { text: `Habis ${-n} hari lalu`, cls: 'bad' }
  if (n <= 60) return { text: `${n} hari lagi`, cls: 'soon' }
  return { text: `${n} hari lagi`, cls: '' }
})

const shareUrl = computed(() => `${lookups.publicBaseUrl}/a/${tag.value}`)

// ---- actions ----
const editVisible = ref(false)
const onSaved = (a: Asset) => {
  asset.value = a
  loadLogs()
}

const FINDING = {
  new_damage: { label: 'Rusak baru', severity: 'danger' },
  existing: { label: 'Sudah rusak', severity: 'warn' },
  fixed: { label: 'Diperbaiki', severity: 'success' },
  battery: { label: 'Baterai turun', severity: 'warn' },
} as const
const whoText = (f: ConditionReport['findings'][number]) =>
  f.responsible.kind === 'employee'
    ? `Pemegang: ${f.responsible.name}`
    : f.responsible.kind === 'storage'
      ? 'Saat di IT / penyimpanan'
      : 'Tidak dapat dipastikan'

const statusVisible = ref(false)
const newStatus = ref<string | null>(null)
const statusNote = ref('')
const statusErr = ref('')
const busy = ref(false)
const statusChoices = computed(() => statusOptions.filter((s) => s.value !== 'in_use' && s.value !== asset.value?.status))

function openStatus() {
  newStatus.value = null
  statusNote.value = ''
  statusErr.value = ''
  statusVisible.value = true
}
async function saveStatus() {
  if (!asset.value || !newStatus.value) {
    statusErr.value = 'Pilih status baru'
    return
  }
  busy.value = true
  try {
    asset.value = (await http.put<Envelope<Asset>>(`/assets/${asset.value.id}/status`, { status: newStatus.value, note: statusNote.value })).data
    notify.success('Status diperbarui menjadi ' + statusLabel(newStatus.value))
    statusVisible.value = false
    await loadLogs()
  } catch (e) {
    statusErr.value = e instanceof ApiError ? e.message : 'Gagal menyimpan'
  } finally {
    busy.value = false
  }
}

const noteVisible = ref(false)
const noteText = ref('')
const noteErr = ref('')
async function saveNote() {
  if (!asset.value) return
  busy.value = true
  noteErr.value = ''
  try {
    await http.post(`/assets/${asset.value.id}/notes`, { message: noteText.value })
    noteVisible.value = false
    noteText.value = ''
    notify.success('Catatan ditambahkan')
    await loadLogs()
  } catch (e) {
    noteErr.value = e instanceof ApiError ? (e.errors.message ?? e.message) : 'Gagal menyimpan'
  } finally {
    busy.value = false
  }
}

function hand() {
  if (!asset.value) return
  if (asset.value.status === 'in_use')
    router.push({ name: 'handover.create', query: { type: 'in', asset: asset.value.id, employee: asset.value.holder_id ?? undefined } })
  else router.push({ name: 'handover.create', query: { type: 'out', asset: asset.value.id } })
}

function remove() {
  if (!asset.value) return
  confirm.require({
    header: 'Hapus Aset',
    message: `Hapus ${asset.value.tag} (${assetTitle(asset.value)})? Aset yang sudah pernah diserahterimakan tidak bisa dihapus — ubah statusnya menjadi Dimusnahkan.`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Hapus',
    rejectLabel: 'Batal',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', text: true },
    accept: async () => {
      try {
        await http.delete(`/assets/${asset.value!.id}`)
        notify.success('Aset dihapus')
        router.replace({ name: 'asset' })
      } catch (e) {
        notify.error(e instanceof ApiError ? e.message : 'Gagal menghapus')
      }
    },
  })
}

// ---- photos ----
const MAX_IMAGES = 5
const uploading = ref(false)
async function onFiles(e: Event) {
  const input = e.target as HTMLInputElement
  if (!asset.value || !input.files?.length) return
  const fd = new FormData()
  Array.from(input.files).forEach((f) => fd.append('images', f))
  input.value = ''
  uploading.value = true
  try {
    asset.value = (await http.post<Envelope<Asset>>(`/assets/${asset.value.id}/images`, fd)).data
    notify.success('Foto ditambahkan')
    await loadLogs()
  } catch (er) {
    notify.fromError(er)
    if (er instanceof ApiError && er.errors.images) notify.error(er.errors.images)
  } finally {
    uploading.value = false
  }
}
function removePhoto(id: number) {
  confirm.require({
    header: 'Hapus Foto',
    message: 'Hapus foto ini?',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Hapus',
    rejectLabel: 'Batal',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', text: true },
    accept: async () => {
      try {
        await http.delete(`/assets/${asset.value!.id}/images/${id}`)
        asset.value!.images = asset.value!.images.filter((i) => i.id !== id)
        await loadLogs()
      } catch (er) {
        notify.fromError(er)
      }
    },
  })
}

async function copyLink() {
  try {
    await navigator.clipboard.writeText(shareUrl.value)
    notify.success('Tautan disalin')
  } catch {
    notify.error('Tidak dapat menyalin, salin manual: ' + shareUrl.value)
  }
}
</script>

<template>
  <Message v-if="notFound" severity="warn" :closable="false">
    Aset dengan kode <b class="mono">{{ tag }}</b> tidak ditemukan.
    <RouterLink :to="{ name: 'asset' }" class="link">Kembali ke daftar aset</RouterLink>
  </Message>
  <Message v-else-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div v-else-if="loading" class="page-card muted">Memuat…</div>

  <template v-else-if="asset">
    <div class="page-card head">
      <div class="title">
        <div class="mono tagline">{{ asset.tag }}</div>
        <h2>{{ assetTitle(asset) }}<span v-if="asset.quantity > 1" class="muted"> ×{{ asset.quantity }}</span></h2>
        <div class="meta">
          <StatusTag kind="asset" :status="asset.status" />
          <span class="muted">{{ asset.category?.name }}</span>
          <span v-if="asset.holder"><i class="pi pi-user" /> {{ asset.holder.name }}<span v-if="asset.holder.department" class="muted"> · {{ asset.holder.department }}</span></span>
          <span v-else-if="asset.location"><i class="pi pi-map-marker" /> {{ asset.location }}</span>
        </div>
      </div>
      <div class="toolbar actions">
        <Button
          v-if="asset.status === 'in_use'"
          label="Kembalikan"
          icon="pi pi-arrow-left"
          @click="hand"
        />
        <Button v-else-if="asset.status === 'available'" label="Serahkan" icon="pi pi-arrow-right" @click="hand" />
        <Button v-if="asset.status !== 'in_use'" label="Ubah Status" icon="pi pi-flag" severity="secondary" outlined @click="openStatus" />
        <Button label="Catatan" icon="pi pi-comment" severity="secondary" outlined @click="noteVisible = true" />
        <Button label="Ubah" icon="pi pi-pencil" severity="secondary" outlined @click="editVisible = true" />
        <Button label="Label" icon="pi pi-print" severity="secondary" outlined @click="router.push({ name: 'labels', query: { ids: asset.id } })" />
        <Button icon="pi pi-trash" severity="danger" text rounded aria-label="Hapus" @click="remove" />
      </div>
    </div>

    <div class="cols">
      <div class="stack">
        <div class="page-card">
          <h3>Informasi</h3>
          <table class="kv">
            <tbody>
              <tr><th>Merk</th><td>{{ dash(asset.brand) }}</td></tr>
              <tr><th>Tipe</th><td>{{ dash(asset.model) }}</td></tr>
              <tr><th>Serial Number</th><td class="mono">{{ dash(asset.serial_number) }}</td></tr>
              <tr v-for="s in specs" :key="s.label"><th>{{ s.label }}</th><td>{{ dash(s.value) }}</td></tr>
              <tr><th>Tahun Pembuatan</th><td>{{ dash(asset.manufacture_year) }}</td></tr>
              <tr><th>Lokasi</th><td>{{ asset.holder ? 'Dipegang ' + asset.holder.name : dash(asset.location) }}</td></tr>
              <tr v-if="asset.notes"><th>Keterangan</th><td class="pre">{{ asset.notes }}</td></tr>
            </tbody>
          </table>
        </div>

        <div class="page-card">
          <h3>Pembelian &amp; Garansi</h3>
          <table class="kv">
            <tbody>
              <tr><th>Tanggal Beli</th><td>{{ formatDate(asset.purchase_date) }}</td></tr>
              <tr><th>Harga Beli</th><td>{{ formatRupiah(asset.purchase_price) }}</td></tr>
              <tr><th>Vendor</th><td>{{ dash(asset.vendor) }}</td></tr>
              <tr>
                <th>Garansi Sampai</th>
                <td>
                  {{ formatDate(asset.warranty_until) }}
                  <span v-if="warranty" class="badge" :class="warranty.cls">{{ warranty.text }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="report?.events.length" class="page-card">
          <h3>Pemeriksaan &amp; Tanggung Jawab</h3>
          <p class="muted small">{{ report.disclaimer }}</p>
          <p v-if="!report.findings.length" class="muted">Belum ada perubahan kondisi yang terdeteksi antar pemeriksaan.</p>
          <ul class="findings">
            <li v-for="(f, i) in report.findings" :key="i">
              <div class="frow">
                <Tag :value="FINDING[f.type].label" :severity="FINDING[f.type].severity" />
                <b>{{ f.label }}</b>
                <span class="who" :class="f.responsible.kind">{{ whoText(f) }}</span>
              </div>
              <div v-if="f.note" class="pre fnote">“{{ f.note }}”</div>
              <div class="small muted">{{ f.explanation }}</div>
            </li>
          </ul>
          <h4>Riwayat pemeriksaan</h4>
          <ul class="events">
            <li v-for="e in [...report.events].reverse()" :key="e.inspection_id">
              <span class="nowrap muted small">{{ formatDateTime(e.at) }}</span>
              <Tag :value="txTypeLabel(e.kind)" severity="secondary" />
              <span>
                <template v-if="e.kind === 'transfer'">{{ e.from_employee?.name }} → {{ e.employee?.name }}</template>
                <template v-else>{{ e.employee?.name }}</template>
              </span>
              <span v-if="e.skipped" class="muted small">Dilewati: {{ e.skip_reason }}</span>
              <template v-else>
                <Tag :value="CONCLUSION[e.conclusion]?.label ?? '-'" :severity="CONCLUSION[e.conclusion]?.severity" />
                <span v-if="e.bad_items.length" class="small bad">{{ e.bad_items.length }} Tidak Baik</span>
                <span v-if="e.battery_health !== null" class="small muted">baterai {{ e.battery_health }}%</span>
              </template>
              <a :href="`/api/transactions/${e.transaction_id}/bast`" target="_blank" rel="noopener" class="link small">BAST {{ e.order_number }}</a>
            </li>
          </ul>
        </div>

        <div class="page-card">
          <div class="row-head">
            <h3>Foto ({{ asset.images.length }}/{{ MAX_IMAGES }})</h3>
            <label v-if="asset.images.length < MAX_IMAGES" class="p-button p-component p-button-outlined p-button-secondary p-button-sm add-photo">
              <i class="pi pi-camera" /> {{ uploading ? 'Mengunggah…' : 'Tambah foto' }}
              <input type="file" accept="image/jpeg,image/png,image/gif" multiple capture="environment" hidden :disabled="uploading" @change="onFiles" />
            </label>
          </div>
          <p v-if="!asset.images.length" class="muted">Belum ada foto. Foto label serial atau kondisi fisik membantu saat audit.</p>
          <div class="photos">
            <div v-for="img in asset.images" :key="img.id" class="photo">
              <a :href="`/api/uploads/assets/${img.name}`" target="_blank" rel="noopener"><img :src="`/api/uploads/assets/${img.name}`" alt="" loading="lazy" /></a>
              <button type="button" aria-label="Hapus foto" @click="removePhoto(img.id)"><i class="pi pi-times" /></button>
            </div>
          </div>
        </div>
      </div>

      <div class="stack">
        <div class="page-card center">
          <h3>Kode Aset</h3>
          <img :src="`/api/assets/${asset.id}/qr?size=300`" :alt="`QR ${asset.tag}`" class="qr" />
          <img :src="`/api/assets/${asset.id}/barcode`" :alt="`Barcode ${asset.tag}`" class="bar" />
          <div class="mono">{{ asset.tag }}</div>
          <div class="muted small url" :title="shareUrl">{{ shareUrl }}</div>
          <div class="toolbar" style="justify-content: center; margin-top: 0.5rem">
            <Button label="Salin tautan" icon="pi pi-copy" size="small" severity="secondary" outlined @click="copyLink" />
            <Button label="Cetak label" icon="pi pi-print" size="small" @click="router.push({ name: 'labels', query: { ids: asset.id } })" />
          </div>
        </div>

        <div class="page-card">
          <h3>Riwayat</h3>
          <p v-if="!logs.length" class="muted">Belum ada riwayat.</p>
          <ul class="timeline">
            <li v-for="l in logs" :key="l.id">
              <i class="pi dot" :class="LOG_ACTIONS[l.action]?.icon ?? 'pi-circle'" />
              <div>
                <div>
                  <b>{{ LOG_ACTIONS[l.action]?.label ?? l.action }}</b>
                  <span v-if="l.from_status && l.to_status && l.from_status !== l.to_status" class="muted">
                    · {{ statusLabel(l.from_status) }} → {{ statusLabel(l.to_status) }}
                  </span>
                </div>
                <div v-if="l.message" class="pre">{{ l.message }}</div>
                <div class="muted small">{{ formatDateTime(l.created_at) }}<span v-if="l.user"> · {{ l.user.name }}</span></div>
              </div>
            </li>
          </ul>
        </div>
      </div>
    </div>

    <AssetForm v-model:visible="editVisible" :asset="asset" @saved="onSaved" />

    <Dialog v-model:visible="statusVisible" modal header="Ubah Status" :style="{ width: '26rem', maxWidth: '95vw' }">
      <p class="muted" style="margin-top: 0">Status sekarang: <b>{{ statusLabel(asset.status) }}</b>. Untuk status Dipakai gunakan Serah Terima.</p>
      <div class="field">
        <label>Status baru</label>
        <Select v-model="newStatus" :options="statusChoices" option-label="label" option-value="value" placeholder="Pilih status" fluid />
      </div>
      <div class="field">
        <label for="sn-note">Catatan (mis. kerusakan, no. tiket servis)</label>
        <Textarea id="sn-note" v-model="statusNote" rows="3" auto-resize fluid />
      </div>
      <small v-if="statusErr" class="field-error">{{ statusErr }}</small>
      <div class="toolbar" style="justify-content: flex-end; margin-top: 0.75rem">
        <Button label="Batal" severity="secondary" text @click="statusVisible = false" />
        <Button label="Simpan" icon="pi pi-check" :loading="busy" @click="saveStatus" />
      </div>
    </Dialog>

    <Dialog v-model:visible="noteVisible" modal header="Tambah Catatan" :style="{ width: '26rem', maxWidth: '95vw' }">
      <div class="field">
        <label for="note">Catatan servis / perbaikan / lainnya</label>
        <Textarea id="note" v-model="noteText" rows="4" auto-resize :invalid="!!noteErr" fluid />
        <small class="muted">Jangan tulis password di sini.</small>
        <small v-if="noteErr" class="field-error">{{ noteErr }}</small>
      </div>
      <div class="toolbar" style="justify-content: flex-end">
        <Button label="Batal" severity="secondary" text @click="noteVisible = false" />
        <Button label="Simpan" icon="pi pi-check" :loading="busy" @click="saveNote" />
      </div>
    </Dialog>
  </template>
</template>

<style scoped>
.head { display: flex; gap: 1rem; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; margin-bottom: 1rem; }
.tagline { font-size: 0.95rem; color: var(--p-primary-color); font-weight: 700; letter-spacing: 0.04em; }
.title h2 { margin: 0.15rem 0 0.5rem; font-size: 1.4rem; }
.meta { display: flex; gap: 0.75rem; flex-wrap: wrap; align-items: center; }
.actions { align-items: center; }
.cols { display: grid; grid-template-columns: minmax(0, 1.5fr) minmax(0, 1fr); gap: 1rem; align-items: start; }
@media (max-width: 1000px) { .cols { grid-template-columns: 1fr; } }
.stack { display: grid; gap: 1rem; }
h3 { margin: 0 0 0.75rem; font-size: 1rem; }
.kv { width: 100%; border-collapse: collapse; }
.kv th { text-align: left; width: 9.5rem; color: var(--p-text-muted-color); font-weight: 500; padding: 0.4rem 0.5rem; vertical-align: top; }
.kv td { padding: 0.4rem 0.5rem; word-break: break-word; }
.kv tr + tr { border-top: 1px solid var(--p-content-border-color); }
.pre { white-space: pre-wrap; word-break: break-word; }
.badge { margin-left: 0.5rem; font-size: 0.75rem; padding: 0.1rem 0.5rem; border-radius: 99px; background: var(--p-surface-100); }
.badge.bad { background: #fee2e2; color: #b91c1c; }
.badge.soon { background: #ffedd5; color: #c2410c; }
.row-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem; }
.row-head h3 { margin: 0; }
.add-photo { cursor: pointer; gap: 0.4rem; }
.photos { display: flex; flex-wrap: wrap; gap: 0.6rem; }
.photo { position: relative; width: 7rem; height: 5.5rem; border-radius: 0.5rem; overflow: hidden; border: 1px solid var(--p-content-border-color); }
.photo img { width: 100%; height: 100%; object-fit: cover; display: block; }
.photo button { position: absolute; top: 0.25rem; right: 0.25rem; width: 1.5rem; height: 1.5rem; border-radius: 50%; border: 0; background: rgb(0 0 0 / 0.65); color: #fff; cursor: pointer; display: grid; place-items: center; font-size: 0.7rem; }
.center { text-align: center; }
h4 { margin: 1.1rem 0 0.5rem; font-size: 0.9rem; }
.findings, .events { list-style: none; margin: 0; padding: 0; display: grid; gap: 0.8rem; }
.frow { display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap; margin-bottom: 0.15rem; }
.who { font-size: 0.8rem; padding: 0.1rem 0.55rem; border-radius: 99px; background: var(--p-surface-100); }
.who.employee { background: #fee2e2; color: #991b1b; }
.who.storage { background: #e0e7ff; color: #3730a3; }
.fnote { font-style: italic; margin: 0.1rem 0; }
.events li { display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap; font-size: 0.88rem; }
.bad { color: var(--p-red-500); }
.qr { width: 11rem; height: 11rem; background: #fff; padding: 0.4rem; border-radius: 0.5rem; image-rendering: pixelated; }
.bar { display: block; margin: 0.5rem auto; max-width: 100%; height: auto; background: #fff; padding: 0.25rem; border-radius: 0.25rem; }
.small { font-size: 0.8rem; }
.url { word-break: break-all; }
.timeline { list-style: none; margin: 0; padding: 0; display: grid; gap: 0.9rem; }
.timeline li { display: flex; gap: 0.75rem; }
.dot { flex: none; width: 1.8rem; height: 1.8rem; border-radius: 50%; display: grid; place-items: center; background: var(--p-surface-100); color: var(--p-primary-color); font-size: 0.8rem; }
.app-dark .dot { background: var(--p-surface-800); }
</style>
