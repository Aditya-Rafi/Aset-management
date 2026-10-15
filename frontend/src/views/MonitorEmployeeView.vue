<script setup lang="ts">
import { onMounted, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { http, type Envelope } from '@/api/http'
import { useServerTable } from '@/composables/useServerTable'
import { useNotify } from '@/composables/useApiAction'
import StatusTag from '@/components/StatusTag.vue'
import { assetTitle, dash, formatDateTime, specSummary } from '@/utils/format'
import type { Asset, Employee, TransactionDetail } from '@/types/models'

interface Row { id: number; name: string; department: string | null; asset_count: number; transaction_count: number }

const notify = useNotify()
const table = useServerTable<Row>('/monitoring/employees')
onMounted(table.load)

const visible = ref(false)
const current = ref<Employee | null>(null)
const assets = ref<Asset[]>([])
const history = ref<TransactionDetail[]>([])
const loading = ref(false)

async function openDetail(r: Row) {
  current.value = null
  assets.value = []
  history.value = []
  visible.value = true
  loading.value = true
  try {
    const [a, h] = await Promise.all([
      http.get<Envelope<{ employee: Employee; assets: Asset[] }>>(`/monitoring/employees/${r.id}/assets`),
      http.get<Envelope<TransactionDetail[]>>(`/monitoring/employees/${r.id}/history`),
    ])
    current.value = a.data.employee
    assets.value = a.data.assets
    history.value = h.data
  } catch (e) {
    notify.fromError(e)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="page-card">
    <div class="page-head">
      <h2>Inventaris per Karyawan</h2>
      <InputText v-model="table.search.value" placeholder="Cari nama atau departemen…" size="small" />
    </div>
    <Message v-if="table.error.value" severity="error" :closable="false" style="margin-bottom: 0.75rem">{{ table.error.value }}</Message>

    <DataTable
      :value="table.rows.value" lazy paginator data-key="id"
      :rows="table.perPage.value" :first="(table.page.value - 1) * table.perPage.value" :total-records="table.total.value"
      :rows-per-page-options="[10, 25, 50, 100]" :loading="table.loading.value"
      :sort-field="table.sortField.value" :sort-order="table.sortOrder.value ?? undefined"
      removable-sort striped-rows size="small"
      paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink CurrentPageReport NextPageLink LastPageLink"
      current-page-report-template="{first}–{last} dari {totalRecords}"
      @page="table.onPage" @sort="table.onSort"
    >
      <template #empty>Tidak ada data</template>
      <Column header="No" style="width: 3.5rem"><template #body="{ index }">{{ (table.page.value - 1) * table.perPage.value + index + 1 }}</template></Column>
      <Column field="name" header="Karyawan" sortable />
      <Column field="department" header="Departemen" sortable><template #body="{ data }">{{ dash(data.department) }}</template></Column>
      <Column field="asset_count" header="Aset Dipegang" sortable>
        <template #body="{ data }"><Tag :value="String(data.asset_count)" :severity="data.asset_count ? 'info' : 'secondary'" /></template>
      </Column>
      <Column field="transaction_count" header="Jumlah Dokumen" sortable />
      <Column header="Aksi" style="width: 8rem">
        <template #body="{ data }"><Button label="Lihat" icon="pi pi-list" size="small" severity="info" text @click="openDetail(data)" /></template>
      </Column>
    </DataTable>
  </div>

  <Dialog v-model:visible="visible" modal header="Inventaris Karyawan" :style="{ width: '60rem', maxWidth: '96vw' }">
    <p v-if="loading" class="muted">Memuat…</p>
    <template v-else-if="current">
      <h3 class="who">{{ current.name }} <span v-if="current.department" class="muted">· {{ current.department }}</span></h3>

      <h4>Sedang dipegang ({{ assets.length }})</h4>
      <DataTable :value="assets" size="small" data-key="id">
        <template #empty>Tidak memegang aset apa pun</template>
        <Column header="Kode"><template #body="{ data }"><RouterLink :to="{ name: 'asset.detail', params: { tag: data.tag } }" class="link mono">{{ data.tag }}</RouterLink></template></Column>
        <Column header="Jenis"><template #body="{ data }">{{ data.category?.name }}</template></Column>
        <Column header="Barang">
          <template #body="{ data }">
            {{ assetTitle(data) }}
            <div class="muted small">{{ specSummary(data) }}</div>
          </template>
        </Column>
        <Column header="Catatan"><template #body="{ data }">{{ dash(data.notes) }}</template></Column>
      </DataTable>

      <h4>Riwayat dokumen</h4>
      <DataTable :value="history" size="small" data-key="id">
        <template #empty>Belum ada dokumen</template>
        <Column header="Nomor"><template #body="{ data }"><span class="mono">{{ data.transaction?.order_number }}</span></template></Column>
        <Column header="Jenis"><template #body="{ data }"><StatusTag v-if="data.transaction" kind="tx" :status="data.transaction.type" /></template></Column>
        <Column header="Aset"><template #body="{ data }"><span class="mono">{{ data.asset?.tag }}</span> {{ data.asset ? assetTitle(data.asset) : '' }}</template></Column>
        <Column header="Tanggal"><template #body="{ data }">{{ formatDateTime(data.transaction?.created_at) }}</template></Column>
      </DataTable>
    </template>
  </Dialog>
</template>

<style scoped>
.who { margin: 0 0 1rem; }
h4 { margin: 1.25rem 0 0.5rem; }
.small { font-size: 0.8rem; }
</style>
