<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { http, ApiError, type Envelope } from '@/api/http'
import { useNotify } from '@/composables/useApiAction'
import { useLookups } from '@/stores/lookups'
import RemoteSelect from '@/components/RemoteSelect.vue'
import AutocompleteInput from '@/components/AutocompleteInput.vue'
import StatusTag from '@/components/StatusTag.vue'
import InspectionDialog from '@/components/InspectionDialog.vue'
import { assetTitle, specSummary, statusLabel, tagFromScan, txTypeLabel } from '@/utils/format'
import type { Asset, Employee, InspectionInput, Option, Transaction, TxType } from '@/types/models'

const route = useRoute()
const router = useRouter()
const notify = useNotify()
const lookups = useLookups()

interface Row { asset: Asset; result_status: string; note: string; inspection: InspectionInput | null }

const type = ref<TxType>(route.query.type === 'in' ? 'in' : route.query.type === 'transfer' ? 'transfer' : 'out')
const employee = ref<Option | null>(null) // out: receiver · in: returner · transfer: new holder
const fromEmployee = ref<Option | null>(null) // transfer: current holder
const note = ref('')
const location = ref('')
const rows = ref<Row[]>([])
const errors = ref<Record<string, string>>({})
const saving = ref(false)
const created = ref<Transaction | null>(null)

const typeOptions = [
  { label: 'Serah Terima (IT → karyawan)', value: 'out' },
  { label: 'Pengembalian (karyawan → IT)', value: 'in' },
  { label: 'Pindah Tangan (karyawan → karyawan)', value: 'transfer' },
]
const resultOptions = [
  { label: 'Tersedia', value: 'available' },
  { label: 'Perlu diservis', value: 'repair' },
  { label: 'Rusak', value: 'broken' },
]
const fromStatus: Record<string, string> = { layak: 'available', perlu_perbaikan: 'repair', tidak_layak: 'broken' }

const holderId = computed(() => (type.value === 'transfer' ? fromEmployee.value?.id : type.value === 'in' ? employee.value?.id : undefined))
const assetFilter = computed(() =>
  type.value === 'out' ? { status: 'available' } : { status: 'in_use', holder_id: holderId.value ? String(holderId.value) : undefined },
)
const profileOf = (a: Asset) => a.category?.inspection ?? 'none'
const needsCheck = (r: Row) => profileOf(r.asset) !== 'none'

/** Why this asset cannot be part of the document being built (null = fine). */
function problem(a: Asset): string | null {
  if (rows.value.some((r) => r.asset.id === a.id)) return `${a.tag} sudah ada di daftar`
  if (type.value === 'out' && a.status !== 'available') return `${a.tag} berstatus ${statusLabel(a.status)}, hanya aset Tersedia yang bisa diserahkan`
  if (type.value !== 'out') {
    if (a.status !== 'in_use') return `${a.tag} tidak sedang dipakai`
    const want = holderId.value
    if (want && a.holder_id !== want) {
      const who = type.value === 'transfer' ? fromEmployee.value?.text : employee.value?.text
      return `${a.tag} dipegang ${a.holder?.name ?? 'orang lain'}, bukan ${who}`
    }
  }
  return null
}

function addAsset(a: Asset) {
  const p = problem(a)
  if (p) {
    notify.error(p)
    return false
  }
  // these documents belong to one holder: take it from the first asset
  if (type.value === 'in' && !employee.value && a.holder) employee.value = { id: a.holder.id, text: a.holder.name }
  if (type.value === 'transfer' && !fromEmployee.value && a.holder) fromEmployee.value = { id: a.holder.id, text: a.holder.name }
  rows.value.push({ asset: a, result_status: 'available', note: '', inspection: null })
  // open the checklist right away: the unit is in front of the IT person
  const added = rows.value[rows.value.length - 1]
  if (needsCheck(added)) openInspection(added)
  return true
}

const picker = ref<Option | null>(null)
async function onPick(opt: Option | null) {
  picker.value = null
  if (!opt) return
  try {
    addAsset((await http.get<Envelope<Asset>>(`/assets/${opt.id}`)).data)
  } catch (e) {
    notify.fromError(e)
  }
}

// USB barcode scanners "type" the code and press Enter; phones can paste the sticker URL.
const code = ref('')
const scanning = ref(false)
async function addByCode() {
  const tag = tagFromScan(code.value)
  if (!tag) return
  scanning.value = true
  try {
    const a = (await http.get<Envelope<Asset>>(`/assets/tag/${encodeURIComponent(tag)}`)).data
    if (addAsset(a)) code.value = ''
  } catch (e) {
    notify.error(e instanceof ApiError && e.status === 404 ? `Kode ${tag} tidak dikenal` : 'Gagal mencari aset')
  } finally {
    scanning.value = false
  }
}

const removeRow = (i: number) => rows.value.splice(i, 1)

watch(type, () => {
  rows.value = []
  employee.value = null
  fromEmployee.value = null
  errors.value = {}
})
watch(employee, (e, old) => {
  if (type.value === 'in' && old && e?.id !== old.id) rows.value = []
})
watch(fromEmployee, (e, old) => {
  if (type.value === 'transfer' && old && e?.id !== old.id) rows.value = []
})

// ---- physical check ----
// The dialog stays mounted once used and is only shown/hidden; unmounting it while open leaves its overlay behind.
const inspecting = ref<Row | null>(null)
const inspectOpen = ref(false)
function openInspection(r: Row) {
  inspecting.value = r
  inspectOpen.value = true
}
function saveInspection(input: InspectionInput) {
  const r = inspecting.value
  if (!r) return
  r.inspection = input
  if (type.value === 'in' && !input.skipped && input.conclusion) r.result_status = fromStatus[input.conclusion] ?? 'available'
}
function checkLabel(r: Row): { text: string; severity: 'success' | 'warn' | 'danger' | 'secondary' } {
  if (!needsCheck(r)) return { text: 'Tidak perlu', severity: 'secondary' }
  const i = r.inspection
  if (!i) return { text: 'Belum diperiksa', severity: 'danger' }
  if (i.skipped) return { text: 'Dilewati', severity: 'warn' }
  const bad = (i.items ?? []).filter((x) => x.result === 'bad').length
  return bad ? { text: `${bad} Tidak Baik`, severity: 'warn' } : { text: 'Semua Baik', severity: 'success' }
}
const unchecked = computed(() => rows.value.filter((r) => needsCheck(r) && !r.inspection))

onMounted(async () => {
  lookups.loadTemplates().catch(() => {})
  const assetId = route.query.asset
  const empId = route.query.employee
  try {
    if (empId) {
      const emp = (await http.get<Envelope<Employee>>(`/employees/${empId}`)).data
      if (type.value === 'transfer') fromEmployee.value = { id: emp.id, text: emp.name }
      else employee.value = { id: emp.id, text: emp.name }
    }
    if (assetId) addAsset((await http.get<Envelope<Asset>>(`/assets/${assetId}`)).data)
  } catch (e) {
    notify.fromError(e)
  }
})

function reset() {
  employee.value = null
  fromEmployee.value = null
  note.value = ''
  location.value = ''
  rows.value = []
  errors.value = {}
  created.value = null
}

async function submit() {
  errors.value = {}
  if (unchecked.value.length) {
    errors.value = { items: `Isi pemeriksaan fisik dulu untuk: ${unchecked.value.map((r) => r.asset.tag).join(', ')}` }
    notify.error(errors.value.items)
    return
  }
  saving.value = true
  try {
    const res = await http.post<Envelope<Transaction>>('/transactions', {
      type: type.value,
      employee_id: employee.value?.id ?? 0,
      from_employee_id: type.value === 'transfer' ? (fromEmployee.value?.id ?? 0) : undefined,
      note: note.value,
      location: type.value === 'in' ? location.value : undefined,
      items: rows.value.map((r) => ({
        asset_id: r.asset.id,
        result_status: type.value === 'in' ? r.result_status : undefined,
        note: r.note,
        inspection: needsCheck(r) ? r.inspection : undefined,
      })),
    })
    notify.success(res.message ?? 'Tersimpan')
    created.value = res.data
  } catch (e) {
    errors.value = notify.fromError(e)
    const msg = errors.value.items ?? errors.value.inspections
    if (e instanceof ApiError && msg) notify.error(msg)
  } finally {
    saving.value = false
  }
}

const bastUrl = computed(() => (created.value ? `/api/transactions/${created.value.id}/bast` : ''))
const pdfUrl = computed(() => (created.value ? `/api/transactions/${created.value.id}/pdf` : ''))
const toLabel = computed(() => (type.value === 'in' ? 'Dikembalikan oleh' : type.value === 'transfer' ? 'Diterima oleh' : 'Diserahkan kepada'))
</script>

<template>
  <div v-if="created" class="page-card done">
    <i class="pi pi-check-circle ok" />
    <h2>{{ txTypeLabel(created.type) }} tercatat</h2>
    <p>Nomor dokumen <b class="mono">{{ created.order_number }}</b> · {{ created.details?.length }} aset</p>
    <p class="muted">
      Cetak <b>BAST</b> untuk ditandatangani (memuat hasil pemeriksaan fisik), atau form ringkas sebagai dokumentasi tertulis.
    </p>
    <div class="toolbar" style="justify-content: center">
      <a :href="bastUrl" target="_blank" rel="noopener"><Button label="Cetak BAST + Pemeriksaan" icon="pi pi-file-pdf" /></a>
      <a :href="pdfUrl" target="_blank" rel="noopener"><Button label="Cetak form ringkas" icon="pi pi-file" severity="secondary" outlined /></a>
      <Button label="Buat dokumen lagi" icon="pi pi-plus" severity="secondary" text @click="reset" />
      <Button label="Riwayat dokumen" icon="pi pi-list" severity="secondary" text @click="router.push({ name: 'handover' })" />
    </div>
  </div>

  <form v-else class="page-card" novalidate @submit.prevent="submit">
    <div class="page-head">
      <h2>Buat Dokumen</h2>
      <div class="toolbar">
        <Button type="button" icon="pi pi-refresh" severity="secondary" outlined aria-label="Reset" @click="reset" />
        <Button type="submit" label="Simpan" icon="pi pi-check" :loading="saving" :disabled="!rows.length" />
      </div>
    </div>

    <div class="field">
      <SelectButton v-model="type" :options="typeOptions" option-label="label" option-value="value" :allow-empty="false" />
    </div>

    <div class="grid-3">
      <div v-if="type === 'transfer'" class="field">
        <label>Diserahkan oleh *</label>
        <RemoteSelect v-model="fromEmployee" endpoint="/select/employees" :params="{ holding: '1' }" placeholder="Karyawan yang memegang aset" :invalid="!!errors.from_employee_id" />
        <small v-if="errors.from_employee_id" class="field-error">{{ errors.from_employee_id }}</small>
      </div>
      <div class="field">
        <label>{{ toLabel }} *</label>
        <RemoteSelect
          v-model="employee"
          endpoint="/select/employees"
          :params="type === 'in' ? { holding: '1' } : undefined"
          :placeholder="type === 'in' ? 'Karyawan yang memegang aset' : 'Pilih karyawan'"
          :invalid="!!errors.employee_id"
        />
        <small v-if="errors.employee_id" class="field-error">{{ errors.employee_id }}</small>
      </div>
      <div class="field">
        <label for="note">Catatan dokumen</label>
        <InputText id="note" v-model="note" maxlength="255" :invalid="!!errors.note" fluid />
        <small v-if="errors.note" class="field-error">{{ errors.note }}</small>
      </div>
      <div v-if="type === 'in'" class="field">
        <label for="loc">Disimpan di</label>
        <AutocompleteInput id="loc" v-model="location" field="location" placeholder="mis. Lemari IT" />
      </div>
    </div>

    <div class="grid-2 add">
      <div class="field">
        <label for="scan"><i class="pi pi-qrcode" /> Scan / ketik kode lalu Enter</label>
        <InputText
          id="scan"
          v-model="code"
          placeholder="LPT-0001 atau tempel tautan stiker"
          :disabled="scanning"
          fluid
          autofocus
          @keydown.enter.prevent="addByCode"
        />
      </div>
      <div class="field">
        <label><i class="pi pi-search" /> Atau cari aset</label>
        <RemoteSelect
          :key="type + (holderId ?? 0)"
          v-model="picker"
          endpoint="/select/assets"
          :params="assetFilter"
          :placeholder="type === 'out' ? 'Cari aset yang Tersedia' : 'Cari aset yang sedang dipegang'"
          @update:model-value="onPick"
        />
      </div>
    </div>

    <Message v-if="errors.items || errors.inspections" severity="error" :closable="false" class="gap">{{ errors.items ?? errors.inspections }}</Message>
    <Message v-else-if="unchecked.length" severity="warn" :closable="false" class="gap">
      {{ unchecked.length }} aset belum diperiksa secara fisik. Klik <b>Periksa</b> pada barisnya. Pemeriksaan inilah yang membuktikan kondisi unit saat berpindah tangan.
    </Message>

    <div class="table-wrap">
      <table class="assets">
        <thead>
          <tr>
            <th>Kode</th><th>Barang</th><th>Status</th><th>Pemeriksaan fisik</th>
            <th v-if="type === 'in'">Setelah kembali</th>
            <th>Catatan</th><th />
          </tr>
        </thead>
        <tbody>
          <tr v-if="!rows.length">
            <td :colspan="type === 'in' ? 7 : 6" class="muted empty">Belum ada aset. Scan stiker atau cari aset di atas.</td>
          </tr>
          <tr v-for="(r, i) in rows" :key="r.asset.id">
            <td class="mono nowrap"><b>{{ r.asset.tag }}</b></td>
            <td>
              {{ assetTitle(r.asset) }}
              <div v-if="specSummary(r.asset)" class="muted small">{{ specSummary(r.asset) }}</div>
              <div v-if="r.asset.holder" class="muted small"><i class="pi pi-user" /> {{ r.asset.holder.name }}</div>
            </td>
            <td><StatusTag kind="asset" :status="r.asset.status" /></td>
            <td class="nowrap">
              <Tag :value="checkLabel(r).text" :severity="checkLabel(r).severity" />
              <Button
                v-if="needsCheck(r)"
                type="button"
                :label="r.inspection ? 'Ubah' : 'Periksa'"
                icon="pi pi-list-check"
                size="small"
                :severity="r.inspection ? 'secondary' : 'primary'"
                :outlined="!!r.inspection"
                class="check-btn"
                @click="openInspection(r)"
              />
            </td>
            <td v-if="type === 'in'">
              <Select v-model="r.result_status" :options="resultOptions" option-label="label" option-value="value" size="small" />
            </td>
            <td><InputText v-model="r.note" maxlength="255" size="small" placeholder="opsional" fluid /></td>
            <td><Button type="button" icon="pi pi-times-circle" severity="danger" text rounded aria-label="Hapus baris" @click="removeRow(i)" /></td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="muted small">{{ rows.length }} aset dalam dokumen.</p>
  </form>

  <InspectionDialog
    v-if="inspecting"
    v-model:visible="inspectOpen"
    :asset="inspecting.asset"
    :kind="type"
    :model-value="inspecting.inspection"
    @save="saveInspection"
  />
</template>

<style scoped>
.add { margin-top: 0.5rem; }
.gap { margin-bottom: 0.75rem; }
.table-wrap { overflow-x: auto; }
.assets { width: 100%; border-collapse: collapse; font-size: 0.9rem; }
.assets th { text-align: left; padding: 0.5rem; color: var(--p-text-muted-color); font-weight: 600; border-bottom: 1px solid var(--p-content-border-color); white-space: nowrap; }
.assets td { padding: 0.5rem; vertical-align: middle; border-bottom: 1px solid var(--p-content-border-color); }
.empty { text-align: center; padding: 1.5rem; }
.small { font-size: 0.8rem; }
.check-btn { margin-left: 0.5rem; }
.done { text-align: center; padding: 2.5rem 1rem; }
.ok { font-size: 3rem; color: var(--p-green-500); }
</style>
