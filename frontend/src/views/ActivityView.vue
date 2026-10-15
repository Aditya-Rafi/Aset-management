<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useServerTable } from '@/composables/useServerTable'
import { LOG_ACTIONS, assetTitle, formatDateTime, statusLabel } from '@/utils/format'
import type { AssetLog } from '@/types/models'

const action = ref<string | null>(null)
const table = useServerTable<AssetLog>('/logs', { perPage: 25, params: () => ({ action: action.value ?? undefined }) })
onMounted(table.load)
watch(action, () => table.refresh())

const actionOptions = [{ label: 'Semua aktivitas', value: null }, ...Object.entries(LOG_ACTIONS).map(([value, v]) => ({ value, label: v.label }))]
</script>

<template>
  <div class="page-card">
    <div class="page-head">
      <h2>Log Aktivitas</h2>
      <div class="toolbar">
        <Select v-model="action" :options="actionOptions" option-label="label" option-value="value" placeholder="Semua aktivitas" size="small" />
        <InputText v-model="table.search.value" placeholder="Cari kode, merk, catatan…" size="small" />
      </div>
    </div>
    <p class="muted">Setiap perubahan pada aset — pendaftaran, edit, status, serah terima, catatan servis — tercatat di sini.</p>
    <Message v-if="table.error.value" severity="error" :closable="false" style="margin-bottom: 0.75rem">{{ table.error.value }}</Message>

    <DataTable
      :value="table.rows.value" lazy paginator data-key="id"
      :rows="table.perPage.value" :first="(table.page.value - 1) * table.perPage.value" :total-records="table.total.value"
      :rows-per-page-options="[25, 50, 100]" :loading="table.loading.value" striped-rows size="small" scrollable
      paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink CurrentPageReport NextPageLink LastPageLink"
      current-page-report-template="{first}–{last} dari {totalRecords}"
      @page="table.onPage"
    >
      <template #empty>Belum ada aktivitas</template>
      <Column header="Waktu"><template #body="{ data }"><span class="nowrap">{{ formatDateTime(data.created_at) }}</span></template></Column>
      <Column header="Aset">
        <template #body="{ data }">
          <RouterLink v-if="data.asset" :to="{ name: 'asset.detail', params: { tag: data.asset.tag } }" class="link mono">{{ data.asset.tag }}</RouterLink>
          <div class="muted small">{{ data.asset ? assetTitle(data.asset) : '' }}</div>
        </template>
      </Column>
      <Column header="Aktivitas">
        <template #body="{ data }"><Tag :value="LOG_ACTIONS[data.action]?.label ?? data.action" severity="secondary" /></template>
      </Column>
      <Column header="Keterangan">
        <template #body="{ data }">
          <span v-if="data.from_status && data.to_status && data.from_status !== data.to_status" class="muted">{{ statusLabel(data.from_status) }} → {{ statusLabel(data.to_status) }}. </span>
          {{ data.message }}
        </template>
      </Column>
      <Column header="Oleh"><template #body="{ data }">{{ data.user?.name ?? '-' }}</template></Column>
    </DataTable>
  </div>
</template>

<style scoped>
.small { font-size: 0.8rem; }
</style>
