<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { http, ApiError, type Envelope } from '@/api/http'
import { useServerTable } from '@/composables/useServerTable'
import { useNotify } from '@/composables/useApiAction'
import StatusTag from '@/components/StatusTag.vue'
import { CONCLUSION, assetTitle, dash, formatDateTime, specSummary } from '@/utils/format'
import type { Transaction } from '@/types/models'

const notify = useNotify()
const confirm = useConfirm()

const typeFilter = ref<string | null>(null)
const table = useServerTable<Transaction>('/transactions', { params: () => ({ type: typeFilter.value ?? undefined }) })
onMounted(table.load)
watch(typeFilter, () => table.refresh())

const typeOptions = [
  { label: 'Semua jenis', value: null },
  { label: 'Serah Terima', value: 'out' },
  { label: 'Pengembalian', value: 'in' },
  { label: 'Pindah Tangan', value: 'transfer' },
]

const detailVisible = ref(false)
const detail = ref<Transaction | null>(null)
const detailLoading = ref(false)

async function openDetail(t: Transaction) {
  detail.value = null
  detailVisible.value = true
  detailLoading.value = true
  try {
    detail.value = (await http.get<Envelope<Transaction>>(`/transactions/${t.id}`)).data
  } catch (e) {
    detailVisible.value = false
    notify.error(e instanceof ApiError ? e.message : 'Gagal memuat detail')
  } finally {
    detailLoading.value = false
  }
}

const openPdf = (t: Transaction) => window.open(`/api/transactions/${t.id}/pdf`, '_blank', 'noopener')
const openBast = (t: Transaction) => window.open(`/api/transactions/${t.id}/bast`, '_blank', 'noopener')
const party = (t: Transaction) => (t.type === 'transfer' ? `${t.from_employee?.name ?? '?'} → ${t.employee?.name ?? '?'}` : dash(t.employee?.name))
const inspectionOf = (t: Transaction, assetId: number) => t.inspections?.find((i) => i.asset_id === assetId)

function undo(t: Transaction) {
  confirm.require({
    header: 'Batalkan Dokumen',
    message: `Batalkan ${t.order_number}? Status dan pemegang semua aset di dokumen ini dikembalikan seperti sebelumnya. Hanya bisa jika aset belum berubah lagi.`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Batalkan dokumen',
    rejectLabel: 'Kembali',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', text: true },
    accept: async () => {
      try {
        const res = await http.delete<Envelope<null>>(`/transactions/${t.id}`)
        notify.success(res.message ?? 'Dibatalkan')
        await table.load()
      } catch (e) {
        notify.error(e instanceof ApiError ? e.message : 'Gagal membatalkan')
      }
    },
  })
}
</script>

<template>
  <div class="page-card">
    <div class="page-head">
      <h2>Riwayat Serah Terima &amp; Pengembalian</h2>
      <div class="toolbar">
        <Select v-model="typeFilter" :options="typeOptions" option-label="label" option-value="value" placeholder="Semua jenis" size="small" />
        <InputText v-model="table.search.value" placeholder="Cari nomor, karyawan, kode aset…" size="small" />
        <RouterLink :to="{ name: 'handover.create' }"><Button label="Buat Dokumen" icon="pi pi-plus" /></RouterLink>
      </div>
    </div>

    <Message v-if="table.error.value" severity="error" :closable="false" style="margin-bottom: 0.75rem">{{ table.error.value }}</Message>

    <DataTable
      :value="table.rows.value"
      lazy
      paginator
      data-key="id"
      :rows="table.perPage.value"
      :first="(table.page.value - 1) * table.perPage.value"
      :total-records="table.total.value"
      :rows-per-page-options="[10, 25, 50, 100]"
      :loading="table.loading.value"
      :sort-field="table.sortField.value"
      :sort-order="table.sortOrder.value ?? undefined"
      removable-sort
      striped-rows
      size="small"
      scrollable
      paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink CurrentPageReport NextPageLink LastPageLink"
      current-page-report-template="{first}–{last} dari {totalRecords}"
      @page="table.onPage"
      @sort="table.onSort"
    >
      <template #empty>Belum ada dokumen</template>
      <Column field="order_number" header="Nomor" sortable>
        <template #body="{ data }"><span class="mono nowrap">{{ data.order_number }}</span></template>
      </Column>
      <Column field="type" header="Jenis" sortable>
        <template #body="{ data }"><StatusTag kind="tx" :status="data.type" /></template>
      </Column>
      <Column field="employee" header="Karyawan" sortable>
        <template #body="{ data }">{{ party(data) }}</template>
      </Column>
      <Column header="Aset">
        <template #body="{ data }">
          <div class="uid-list">
            <RouterLink v-for="d in data.details" :key="d.id" :to="{ name: 'asset.detail', params: { tag: d.asset?.tag } }" class="tagchip mono">{{ d.asset?.tag }}</RouterLink>
          </div>
        </template>
      </Column>
      <Column field="note" header="Catatan" sortable>
        <template #body="{ data }">{{ dash(data.note) }}</template>
      </Column>
      <Column field="created_at" header="Tanggal" sortable>
        <template #body="{ data }"><span class="nowrap">{{ formatDateTime(data.created_at) }}</span></template>
      </Column>
      <Column header="Aksi" style="width: 11rem">
        <template #body="{ data }">
          <div class="toolbar nowrap">
            <Button v-tooltip.top="'Lihat'" icon="pi pi-eye" severity="secondary" text rounded size="small" aria-label="Lihat" @click="openDetail(data)" />
            <Button v-tooltip.top="'Cetak BAST + pemeriksaan'" icon="pi pi-file-pdf" severity="info" text rounded size="small" aria-label="Cetak BAST" @click="openBast(data)" />
            <Button v-tooltip.top="'Cetak form ringkas'" icon="pi pi-file" severity="secondary" text rounded size="small" aria-label="Cetak form ringkas" @click="openPdf(data)" />
            <Button v-tooltip.top="'Batalkan'" icon="pi pi-undo" severity="danger" text rounded size="small" aria-label="Batalkan" @click="undo(data)" />
          </div>
        </template>
      </Column>
    </DataTable>
  </div>

  <Dialog v-model:visible="detailVisible" modal header="Detail Dokumen" :style="{ width: '52rem', maxWidth: '96vw' }">
    <p v-if="detailLoading" class="muted">Memuat…</p>
    <template v-else-if="detail">
      <div class="grid-3 info">
        <div><div class="muted">Nomor</div><b class="mono">{{ detail.order_number }}</b></div>
        <div><div class="muted">Jenis</div><StatusTag kind="tx" :status="detail.type" /></div>
        <div><div class="muted">Tanggal</div>{{ formatDateTime(detail.created_at) }}</div>
        <div v-if="detail.type === 'transfer'"><div class="muted">Diserahkan oleh</div>{{ dash(detail.from_employee?.name) }}</div>
        <div><div class="muted">{{ detail.type === 'transfer' ? 'Diterima oleh' : 'Karyawan' }}</div>{{ dash(detail.employee?.name) }}</div>
        <div><div class="muted">Dibuat oleh</div>{{ dash(detail.created_by?.name) }}</div>
        <div><div class="muted">Catatan</div>{{ dash(detail.note) }}</div>
      </div>
      <DataTable :value="detail.details ?? []" size="small" data-key="id">
        <Column header="Kode"><template #body="{ data }"><span class="mono">{{ data.asset?.tag }}</span></template></Column>
        <Column header="Barang">
          <template #body="{ data }">
            {{ data.asset ? assetTitle(data.asset) : '-' }}
            <div class="muted small">{{ data.asset ? specSummary(data.asset) : '' }}</div>
          </template>
        </Column>
        <Column header="Status setelahnya"><template #body="{ data }"><StatusTag kind="asset" :status="data.new_status" /></template></Column>
        <Column header="Pemeriksaan fisik">
          <template #body="{ data }">
            <template v-if="inspectionOf(detail, data.asset_id)">
              <span v-if="inspectionOf(detail, data.asset_id)?.skipped" class="muted">Dilewati: {{ inspectionOf(detail, data.asset_id)?.skip_reason }}</span>
              <template v-else>
                <Tag
                  :value="CONCLUSION[inspectionOf(detail, data.asset_id)?.data.conclusion ?? '']?.label ?? '-'"
                  :severity="CONCLUSION[inspectionOf(detail, data.asset_id)?.data.conclusion ?? '']?.severity"
                />
                <div v-for="b in inspectionOf(detail, data.asset_id)?.data.items.filter((x) => x.result === 'bad')" :key="b.code" class="small bad">
                  Tidak Baik: {{ b.note }}
                </div>
              </template>
            </template>
            <span v-else class="muted">-</span>
          </template>
        </Column>
        <Column header="Catatan"><template #body="{ data }">{{ dash(data.note) }}</template></Column>
      </DataTable>
    </template>
  </Dialog>
</template>

<style scoped>
.info { margin-bottom: 1.25rem; row-gap: 1rem; }
.info .muted { font-size: 0.8rem; }
.small { font-size: 0.8rem; }
.bad { color: var(--p-red-500); }
.tagchip { background: var(--p-surface-100); color: var(--p-text-color); border-radius: 0.35rem; padding: 0.1rem 0.45rem; font-size: 0.78rem; text-decoration: none; }
.tagchip:hover { background: var(--p-primary-50); color: var(--p-primary-700); }
.app-dark .tagchip { background: var(--p-surface-800); }
</style>
