<script setup lang="ts">
import MasterCrud from '@/components/MasterCrud.vue'
</script>

<template>
  <MasterCrud
    title="Daftar Karyawan"
    singular="Karyawan"
    endpoint="/employees"
    :columns="[
      { field: 'name', header: 'Nama', sortKey: 'name' },
      { field: 'nik', header: 'NIK / ID', sortKey: 'nik' },
      { field: 'department', header: 'Departemen', sortKey: 'department' },
      { field: 'hp', header: 'HP', sortKey: 'hp' },
      { field: 'email', header: 'Email', sortKey: 'email' },
    ]"
    :fields="[
      { key: 'name', label: 'Nama Karyawan', required: true },
      { key: 'nik', label: 'NIK / ID Karyawan', help: 'Dicetak pada Berita Acara Serah Terima' },
      { key: 'department', label: 'Departemen / Divisi' },
      { key: 'hp', label: 'HP' },
      { key: 'email', label: 'Email', type: 'email' },
      { key: 'address', label: 'Catatan / Alamat', type: 'textarea' },
    ]"
  />
</template>
