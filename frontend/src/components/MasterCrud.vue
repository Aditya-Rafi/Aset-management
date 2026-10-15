<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Checkbox from 'primevue/checkbox'
import Select from 'primevue/select'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import { useConfirm } from 'primevue/useconfirm'
import { http, ApiError, type Envelope } from '@/api/http'
import { useServerTable } from '@/composables/useServerTable'
import { useNotify } from '@/composables/useApiAction'
import { formatDateTime } from '@/utils/format'
import RemoteSelect from '@/components/RemoteSelect.vue'
import type { Option } from '@/types/models'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Row = { id: number; created_at: string; [k: string]: any }

export interface FieldDef {
  key: string
  label: string
  type?: 'text' | 'email' | 'url' | 'textarea' | 'remote' | 'checkboxes' | 'select'
  required?: boolean
  help?: string
  uppercase?: boolean
  maxlength?: number
  /** checkboxes fields: the choices (value is stored in an array) */
  options?: { label: string; value: string }[]
  /** text fields: suggestions come from /suggest?field=… */
  /** remote fields: lookup endpoint and how to build the initial option from a row */
  endpoint?: string
  optionFrom?(row: any): Option | null
}
export interface ColumnDef {
  field: string
  header: string
  sortKey?: string
  value?(row: any): string | number | null | undefined
}

const props = defineProps<{
  title: string
  singular: string
  endpoint: string
  columns: ColumnDef[]
  fields: FieldDef[]
}>()

const notify = useNotify()
const confirm = useConfirm()
const table = useServerTable<Row>(props.endpoint)
onMounted(table.load)

const visible = ref(false)
const editing = ref<Row | null>(null)
const saving = ref(false)
const errors = ref<Record<string, string>>({})
const values = reactive<Record<string, string | string[] | Option | null>>({})

function blank() {
  for (const f of props.fields) values[f.key] = f.type === 'remote' ? null : f.type === 'checkboxes' ? [] : f.type === 'select' ? (f.options?.[0]?.value ?? '') : ''
}

function openCreate() {
  editing.value = null
  blank()
  errors.value = {}
  visible.value = true
}

function openEdit(row: Row) {
  editing.value = row
  errors.value = {}
  for (const f of props.fields) {
    const raw = (row as Record<string, unknown>)[f.key]
    values[f.key] =
      f.type === 'remote' ? (f.optionFrom?.(row) ?? null) : f.type === 'checkboxes' ? [...((raw as string[]) ?? [])] : String(raw ?? '')
  }
  visible.value = true
}

const dialogTitle = computed(() => (editing.value ? 'Ubah ' : 'Tambah ') + props.singular)

function payload() {
  const out: Record<string, unknown> = {}
  for (const f of props.fields) {
    const v = values[f.key]
    if (f.type === 'remote') out[f.key] = (v as Option | null)?.id ?? 0
    else if (f.type === 'checkboxes') out[f.key] = v as string[]
    else out[f.key] = (v as string).trim() === '' ? null : (v as string).trim()
  }
  // required text fields are sent as '' so the server reports a clear error
  for (const f of props.fields) if (f.required && f.type !== 'remote' && out[f.key] === null) out[f.key] = ''
  return out
}

async function save() {
  saving.value = true
  errors.value = {}
  try {
    const res = editing.value
      ? await http.put<Envelope<Row>>(`${props.endpoint}/${editing.value.id}`, payload())
      : await http.post<Envelope<Row>>(props.endpoint, payload())
    notify.success(res.message ?? 'Tersimpan')
    visible.value = false
    await (editing.value ? table.load() : table.refresh())
  } catch (e) {
    errors.value = notify.fromError(e)
  } finally {
    saving.value = false
  }
}

function remove(row: Row) {
  confirm.require({
    header: `Hapus ${props.singular}`,
    message: `Yakin ingin menghapus ${props.singular.toLowerCase()} ini? Tindakan ini tidak dapat dibatalkan.`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'Hapus',
    rejectLabel: 'Batal',
    acceptProps: { severity: 'danger' },
    rejectProps: { severity: 'secondary', text: true },
    accept: async () => {
      try {
        const res = await http.delete<Envelope<null>>(`${props.endpoint}/${row.id}`)
        notify.success(res.message ?? 'Terhapus')
        await table.load()
      } catch (e) {
        if (e instanceof ApiError) notify.error(e.message)
        else notify.error('Gagal menghapus')
      }
    },
  })
}

const cellValue = (row: Row, c: ColumnDef) => {
  const v = c.value ? c.value(row) : (row as Record<string, unknown>)[c.field]
  return v === null || v === undefined || v === '' ? '-' : v
}
</script>

<template>
  <div class="page-card">
    <div class="page-head">
      <h2>{{ title }}</h2>
      <div class="toolbar">
        <InputText v-model="table.search.value" :placeholder="`Cari ${singular.toLowerCase()}...`" size="small" />
        <Button label="Tambah" icon="pi pi-plus" @click="openCreate" />
      </div>
    </div>

    <Message v-if="table.error.value" severity="error" :closable="false" class="mb">{{ table.error.value }}</Message>

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
      paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink CurrentPageReport NextPageLink LastPageLink"
      current-page-report-template="{first}–{last} dari {totalRecords}"
      @page="table.onPage"
      @sort="table.onSort"
    >
      <template #empty>Tidak ada data</template>
      <Column header="No" style="width: 3.5rem">
        <template #body="{ index }">{{ (table.page.value - 1) * table.perPage.value + index + 1 }}</template>
      </Column>
      <Column v-for="c in columns" :key="c.field" :header="c.header" :field="c.sortKey ?? c.field" :sortable="!!c.sortKey">
        <template #body="{ data }">{{ cellValue(data, c) }}</template>
      </Column>
      <Column field="created_at" header="Created At" sortable>
        <template #body="{ data }">{{ formatDateTime(data.created_at) }}</template>
      </Column>
      <Column header="Aksi" style="width: 7rem">
        <template #body="{ data }">
          <div class="toolbar">
            <Button v-tooltip.top="'Ubah'" icon="pi pi-pencil" severity="info" text rounded size="small" aria-label="Ubah" @click="openEdit(data)" />
            <Button v-tooltip.top="'Hapus'" icon="pi pi-trash" severity="danger" text rounded size="small" aria-label="Hapus" @click="remove(data)" />
          </div>
        </template>
      </Column>
    </DataTable>

    <Dialog v-model:visible="visible" modal :header="dialogTitle" :style="{ width: '32rem', maxWidth: '95vw' }">
      <form novalidate @submit.prevent="save">
        <div v-for="f in fields" :key="f.key" class="field">
          <label :for="'f-' + f.key">{{ f.label }}<span v-if="f.required"> *</span></label>
          <RemoteSelect
            v-if="f.type === 'remote'"
            :model-value="values[f.key] as Option | null"
            :endpoint="f.endpoint!"
            :invalid="!!errors[f.key]"
            :placeholder="'Pilih ' + f.label"
            @update:model-value="values[f.key] = $event"
          />
          <Select
            v-else-if="f.type === 'select'"
            v-model="values[f.key] as string"
            :options="f.options"
            option-label="label"
            option-value="value"
            fluid
          />
          <div v-else-if="f.type === 'checkboxes'" class="checks">
            <label v-for="o in f.options" :key="o.value" class="check">
              <Checkbox v-model="values[f.key] as string[]" :value="o.value" />
              <span>{{ o.label }}</span>
            </label>
          </div>
          <Textarea v-else-if="f.type === 'textarea'" :id="'f-' + f.key" v-model="values[f.key] as string" rows="3" auto-resize :invalid="!!errors[f.key]" fluid />
          <InputText
            v-else
            :id="'f-' + f.key"
            v-model="values[f.key] as string"
            :type="f.type ?? 'text'"
            :maxlength="f.maxlength"
            :class="{ upper: f.uppercase }"
            :invalid="!!errors[f.key]"
            fluid
          />
          <small v-if="f.help && !errors[f.key]" class="muted">{{ f.help }}</small>
          <small v-if="errors[f.key]" class="field-error">{{ errors[f.key] }}</small>
        </div>
        <div class="toolbar" style="justify-content: flex-end">
          <Button type="button" label="Batal" severity="secondary" text @click="visible = false" />
          <Button type="submit" label="Simpan" icon="pi pi-check" :loading="saving" />
        </div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
.mb { margin-bottom: 0.75rem; }
.checks { display: flex; flex-wrap: wrap; gap: 0.5rem 1.25rem; }
.check { display: flex; align-items: center; gap: 0.5rem; font-weight: 400 !important; cursor: pointer; }
.upper { text-transform: uppercase; }
</style>
