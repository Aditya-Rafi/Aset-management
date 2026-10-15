<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Message from 'primevue/message'
import { useServerTable } from '@/composables/useServerTable'
import { useLookups } from '@/stores/lookups'
import { qs } from '@/api/http'
import StatusTag from '@/components/StatusTag.vue'
import AssetForm from '@/components/AssetForm.vue'
import ImportDialog from '@/components/ImportDialog.vue'
import { assetTitle, daysUntil, formatDate, specSummary, statusOptions } from '@/utils/format'
import type { Asset } from '@/types/models'

const route = useRoute()
const router = useRouter()
const lookups = useLookups()

const statusFilter = ref<string | null>((route.query.status as string) || null)
const categoryFilter = ref<number | null>(route.query.category_id ? Number(route.query.category_id) : null)
const filterParams = () => ({ status: statusFilter.value ?? undefined, category_id: categoryFilter.value ?? undefined })
const table = useServerTable<Asset>('/assets', { params: filterParams })
if (typeof route.query.search === 'string') table.search.value = route.query.search

onMounted(() => {
  lookups.loadCategories().catch(() => {})
  table.load()
})
watch([statusFilter, categoryFilter], () => table.refresh())

const selection = ref<Asset[]>([])
const formVisible = ref(false)
const editing = ref<Asset | null>(null)
const importVisible = ref(false)

const openCreate = () => { editing.value = null; formVisible.value = true }
const openEdit = (a: Asset) => { editing.value = a; formVisible.value = true }
const open = (a: Asset) => router.push({ name: 'asset.detail', params: { tag: a.tag } })
const printLabels = (list: Asset[]) => router.push({ name: 'labels', query: { ids: list.map((a) => a.id).join(',') } })
const exportUrl = computed(() => '/api/assets/export' + qs({ search: table.search.value, ...filterParams() }))

function warrantyClass(d?: string | null) {
  const n = daysUntil(d)
  if (n === null) return ''
  return n < 0 ? 'warn-expired' : n <= 60 ? 'warn-soon' : ''
}
const statusSelect = [{ value: null, label: 'Semua status' }, ...statusOptions]
</script>

<template>
  <div class="page-card">
    <div class="page-head">
      <h2>Daftar Aset</h2>
      <div class="toolbar">
        <Select v-model="categoryFilter" :options="lookups.categories" option-label="name" option-value="id" placeholder="Semua kategori" show-clear size="small" />
        <Select v-model="statusFilter" :options="statusSelect" option-label="label" option-value="value" placeholder="Semua status" size="small" />
        <InputText v-model="table.search.value" placeholder="Cari kode, merk, serial, pemegang…" size="small" />
      </div>
    </div>
    <div class="toolbar actions">
      <Button label="Tambah Aset" icon="pi pi-plus" @click="openCreate" />
      <Button label="Impor" icon="pi pi-upload" severity="secondary" outlined @click="importVisible = true" />
      <a :href="exportUrl" download><Button label="Ekspor CSV" icon="pi pi-download" severity="secondary" outlined /></a>
      <Button
        :label="selection.length ? `Cetak Label (${selection.length})` : 'Cetak Label'"
        icon="pi pi-print"
        severity="secondary"
        outlined
        :disabled="!selection.length"
        @click="printLabels(selection)"
      />
    </div>

    <Message v-if="table.error.value" severity="error" :closable="false" class="mb">{{ table.error.value }}</Message>

    <DataTable
      v-model:selection="selection"
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
      <template #empty>Tidak ada aset</template>
      <Column selection-mode="multiple" style="width: 3rem" />
      <Column field="tag" header="Kode" sortable>
        <template #body="{ data }">
          <RouterLink :to="{ name: 'asset.detail', params: { tag: data.tag } }" class="link mono nowrap">{{ data.tag }}</RouterLink>
        </template>
      </Column>
      <Column field="category" header="Kategori" sortable>
        <template #body="{ data }">{{ data.category?.name }}</template>
      </Column>
      <Column field="brand" header="Barang" sortable>
        <template #body="{ data }">
          <div class="clickable" @click="open(data)">
            <b>{{ assetTitle(data) }}</b><span v-if="data.quantity > 1" class="muted"> ×{{ data.quantity }}</span>
            <div v-if="specSummary(data)" class="muted small">{{ specSummary(data) }}</div>
          </div>
        </template>
      </Column>
      <Column field="holder" header="Pemegang / Lokasi" sortable>
        <template #body="{ data }">
          <span v-if="data.holder"><i class="pi pi-user small" /> {{ data.holder.name }}</span>
          <span v-else-if="data.location"><i class="pi pi-map-marker small" /> {{ data.location }}</span>
          <span v-else class="muted">-</span>
        </template>
      </Column>
      <Column field="status" header="Status" sortable>
        <template #body="{ data }"><StatusTag kind="asset" :status="data.status" /></template>
      </Column>
      <Column field="warranty_until" header="Garansi" sortable>
        <template #body="{ data }"><span :class="warrantyClass(data.warranty_until)" class="nowrap">{{ formatDate(data.warranty_until) }}</span></template>
      </Column>
      <Column header="Aksi" style="width: 9rem">
        <template #body="{ data }">
          <div class="toolbar nowrap">
            <Button v-tooltip.top="'Detail'" icon="pi pi-eye" severity="secondary" text rounded size="small" aria-label="Detail" @click="open(data)" />
            <Button v-tooltip.top="'Ubah'" icon="pi pi-pencil" severity="info" text rounded size="small" aria-label="Ubah" @click="openEdit(data)" />
            <Button v-tooltip.top="'Cetak label'" icon="pi pi-print" severity="secondary" text rounded size="small" aria-label="Cetak label" @click="printLabels([data])" />
          </div>
        </template>
      </Column>
    </DataTable>
  </div>

  <AssetForm v-model:visible="formVisible" :asset="editing" @saved="table.load()" />
  <ImportDialog v-model:visible="importVisible" @imported="table.refresh()" />
</template>

<style scoped>
.actions { margin-bottom: 1rem; }
.mb { margin-bottom: 0.75rem; }
.small { font-size: 0.8rem; }
.warn-expired { color: var(--p-red-500); font-weight: 600; }
.warn-soon { color: var(--p-orange-500); font-weight: 600; }
</style>
