<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import RemoteSelect from '@/components/RemoteSelect.vue'
import { http, qs, type Paged } from '@/api/http'
import { useLookups } from '@/stores/lookups'
import { assetTitle, specSummary } from '@/utils/format'
import type { Asset, Option } from '@/types/models'

const route = useRoute()
const router = useRouter()
const lookups = useLookups()

const assets = ref<Asset[]>([])
const loading = ref(false)
const error = ref('')

const ids = computed(() =>
  String(route.query.ids ?? '').split(',').map((s) => Number(s)).filter((n) => n > 0),
)

async function load() {
  if (!ids.value.length) {
    assets.value = []
    return
  }
  loading.value = true
  error.value = ''
  try {
    const res = await http.get<Paged<Asset>>('/assets' + qs({ ids: ids.value.join(','), per_page: 100, sort: 'tag', order: 'asc' }))
    assets.value = res.data
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Gagal memuat aset'
  } finally {
    loading.value = false
  }
}
onMounted(() => {
  lookups.loadConfig().catch(() => {})
  load()
})
watch(ids, load)

// ---- layout presets ----
const presets = {
  s50x30: { label: 'Stiker 50 × 30 mm (1 per halaman)', w: 50, h: 30, page: '50mm 30mm', sheet: false, qr: 22 },
  s70x40: { label: 'Stiker 70 × 40 mm (1 per halaman)', w: 70, h: 40, page: '70mm 40mm', sheet: false, qr: 30 },
  a4: { label: 'Lembar A4 — 3 × 8 label (70 × 37 mm)', w: 70, h: 37, page: 'A4', sheet: true, qr: 28 },
} as const
type PresetKey = keyof typeof presets
const presetKey = ref<PresetKey>('a4')
const preset = computed(() => presets[presetKey.value])
const presetOptions = (Object.keys(presets) as PresetKey[]).map((k) => ({ value: k, label: presets[k].label }))

const showName = ref(true)
const showBarcode = ref(false)

// the paper size has to be set with @page, which cannot be bound from a template
const pageCss = computed(() => `@page { size: ${preset.value.page}; margin: 0; }`)
const printNow = () => window.print()

// the picker adds assets to the print list
const pick = ref<Option | null>(null)
function onPick(o: Option | null) {
  pick.value = null
  if (!o || ids.value.includes(o.id)) return
  router.replace({ query: { ids: [...ids.value, o.id].join(',') } })
}
function removeId(id: number) {
  router.replace({ query: { ids: ids.value.filter((x) => x !== id).join(',') } })
}

const host = computed(() => {
  try {
    return new URL(lookups.publicBaseUrl).hostname
  } catch {
    return ''
  }
})
const localOnly = computed(() => ['localhost', '127.0.0.1', '::1', '[::1]'].includes(host.value))
const sample = computed(() => `${lookups.publicBaseUrl}/a/${assets.value[0]?.tag ?? 'LPT-0001'}`)
</script>

<template>
  <component :is="'style'">{{ pageCss }}</component>

  <div class="page-card no-print">
    <div class="page-head">
      <h2>Cetak Label QR</h2>
      <Button label="Cetak" icon="pi pi-print" :disabled="!assets.length" @click="printNow" />
    </div>

    <Message v-if="localOnly" severity="warn" :closable="false" class="gap">
      QR akan berisi alamat <b class="mono">{{ lookups.publicBaseUrl }}</b> — alamat ini hanya bisa dibuka dari komputer ini, bukan dari HP.
      Atur <span class="mono">PUBLIC_BASE_URL</span> di <span class="mono">backend/.env</span> ke alamat yang bisa dijangkau HP
      (mis. <span class="mono">http://192.168.1.10:8090</span> atau domain Anda), restart server, lalu cetak ulang.
    </Message>
    <Message v-else severity="info" :closable="false" class="gap">
      QR berisi tautan seperti <span class="mono">{{ sample }}</span>. Scan dengan kamera HP (atau Google Lens) akan membuka data aset; pertama kali diminta login.
    </Message>

    <div class="grid-3">
      <div class="field">
        <label>Ukuran label</label>
        <Select v-model="presetKey" :options="presetOptions" option-label="label" option-value="value" fluid />
      </div>
      <div class="field">
        <label>Tambah aset ke daftar cetak</label>
        <RemoteSelect v-model="pick" endpoint="/select/assets" placeholder="Cari kode / merk" @update:model-value="onPick" />
      </div>
      <div class="field opts">
        <label class="check"><Checkbox v-model="showName" binary /> <span>Tampilkan nama barang</span></label>
        <label class="check"><Checkbox v-model="showBarcode" binary /> <span>Tampilkan barcode Code 128</span></label>
      </div>
    </div>

    <div v-if="assets.length" class="chips">
      <span v-for="a in assets" :key="a.id" class="chip mono">
        {{ a.tag }} <button type="button" :aria-label="'Hapus ' + a.tag" @click="removeId(a.id)"><i class="pi pi-times" /></button>
      </span>
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <p v-if="!assets.length && !loading" class="muted">
      Belum ada aset dipilih. Pilih aset di <RouterLink :to="{ name: 'asset' }" class="link">Daftar Aset</RouterLink> (centang lalu “Cetak Label”) atau tambahkan lewat kolom di atas.
    </p>
  </div>

  <div class="sheet" :class="{ a4: preset.sheet }">
    <div v-for="a in assets" :key="a.id" class="label" :style="{ width: preset.w + 'mm', height: preset.h + 'mm' }">
      <img class="qr" :style="{ width: preset.qr + 'mm', height: preset.qr + 'mm' }" :src="`/api/assets/${a.id}/qr?size=400`" alt="" />
      <div class="txt">
        <div class="tag mono">{{ a.tag }}</div>
        <div v-if="showName" class="name">{{ assetTitle(a) }}</div>
        <div v-if="showName && specSummary(a)" class="spec">{{ specSummary(a) }}</div>
        <img v-if="showBarcode" class="bc" :src="`/api/assets/${a.id}/barcode?size=300`" alt="" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.gap { margin-bottom: 1rem; }
.opts { justify-content: flex-end; gap: 0.6rem; }
.check { display: flex; align-items: center; gap: 0.5rem; font-weight: 400 !important; cursor: pointer; }
.chips { display: flex; flex-wrap: wrap; gap: 0.4rem; }
.chip { background: var(--p-surface-100); border-radius: 99px; padding: 0.15rem 0.3rem 0.15rem 0.7rem; font-size: 0.8rem; display: inline-flex; align-items: center; gap: 0.25rem; }
.app-dark .chip { background: var(--p-surface-800); }
.chip button { border: 0; background: transparent; cursor: pointer; color: var(--p-text-muted-color); width: 1.3rem; height: 1.3rem; border-radius: 50%; }
.chip button:hover { background: rgb(0 0 0 / 0.1); }

.sheet { display: flex; flex-wrap: wrap; gap: 4mm; margin-top: 1rem; }
.sheet.a4 { gap: 0; width: 210mm; }
.label {
  box-sizing: border-box; display: flex; align-items: center; gap: 2mm; padding: 2mm;
  background: #fff; color: #000; border: 1px dashed #bbb; overflow: hidden; page-break-inside: avoid; break-inside: avoid;
}
.qr { flex: none; image-rendering: pixelated; }
.txt { min-width: 0; line-height: 1.15; }
.tag { font-weight: 700; font-size: 11pt; }
.name { font-size: 7.5pt; margin-top: 0.5mm; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
.spec { font-size: 6pt; color: #333; margin-top: 0.5mm; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
.bc { max-width: 100%; height: 6mm; margin-top: 1mm; object-fit: contain; object-position: left; }

@media print {
  .sheet { margin: 0; gap: 0; }
  .sheet:not(.a4) .label { page-break-after: always; border: 0; }
  .sheet.a4 .label { border: 0; }
}
</style>
