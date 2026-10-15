<script setup lang="ts">
import MasterCrud from '@/components/MasterCrud.vue'
import { INSPECTION_PROFILES, SPEC_LABELS } from '@/utils/format'
import type { Category } from '@/types/models'

const specOptions = Object.entries(SPEC_LABELS).map(([value, label]) => ({ value, label }))
const inspectionText = (c: Category) => INSPECTION_PROFILES.find((p) => p.value === c.inspection)?.label ?? '-'
const specText = (c: Category) => c.spec_fields.map((f) => SPEC_LABELS[f]).join(', ')
</script>

<template>
  <MasterCrud
    title="Kategori Aset"
    singular="Kategori"
    endpoint="/categories"
    :columns="[
      { field: 'name', header: 'Nama', sortKey: 'name' },
      { field: 'code', header: 'Kode Stiker', sortKey: 'code' },
      { field: 'spec_fields', header: 'Kolom Spesifikasi', value: specText },
      { field: 'inspection', header: 'Pemeriksaan Fisik', value: inspectionText },
    ]"
    :fields="[
      { key: 'name', label: 'Nama Kategori', required: true },
      { key: 'code', label: 'Kode (awalan pada label)', required: true, uppercase: true, maxlength: 5, help: '2–5 huruf, mis. LPT → LPT-0001. Aset yang sudah ada tidak berubah kodenya.' },
      { key: 'spec_fields', label: 'Kolom spesifikasi yang ditampilkan', type: 'checkboxes', options: specOptions },
      { key: 'inspection', label: 'Pemeriksaan fisik saat serah terima', type: 'select', options: INSPECTION_PROFILES, help: 'Checklist yang diisi IT Support setiap kali aset diserahkan, dikembalikan, atau dipindahtangankan' },
    ]"
  />
</template>
