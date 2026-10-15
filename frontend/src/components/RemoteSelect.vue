<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import Select from 'primevue/select'
import { http, qs } from '@/api/http'
import type { Option } from '@/types/models'

const props = withDefaults(
  defineProps<{
    modelValue: Option | null
    endpoint: string
    placeholder?: string
    invalid?: boolean
    /** let the user type a name that does not exist yet (returned with isNew) */
    allowCreate?: boolean
    params?: Record<string, string | undefined>
    disabled?: boolean
  }>(),
  { placeholder: 'Pilih...' },
)
const emit = defineEmits<{ 'update:modelValue': [Option | null] }>()

const fetched = ref<Option[]>([])
const loading = ref(false)
const typed = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let seq = 0

async function search(term: string) {
  const mine = ++seq
  loading.value = true
  try {
    const res = await http.get<Option[]>(props.endpoint + qs({ search: term, ...props.params }))
    if (mine === seq) fetched.value = res
  } catch {
    if (mine === seq) fetched.value = []
  } finally {
    if (mine === seq) loading.value = false
  }
}

function onFilter(e: { value: string }) {
  typed.value = e.value ?? ''
  clearTimeout(timer)
  timer = setTimeout(() => search(typed.value), 250)
}
onBeforeUnmount(() => clearTimeout(timer))

const options = computed<Option[]>(() => {
  const list = [...fetched.value]
  const name = typed.value.trim()
  if (props.allowCreate && name && !list.some((o) => o.text.toLowerCase() === name.toLowerCase())) {
    list.push({ id: 0, text: name, isNew: true })
  }
  const sel = props.modelValue
  if (sel && !list.some((o) => o.id === sel.id && !!o.isNew === !!sel.isNew)) list.unshift(sel)
  return list
})
</script>

<template>
  <Select
    :model-value="modelValue"
    :options="options"
    option-label="text"
    data-key="id"
    filter
    show-clear
    fluid
    :loading="loading"
    :placeholder="placeholder"
    :invalid="invalid"
    :disabled="disabled"
    filter-placeholder="Cari..."
    :empty-filter-message="allowCreate ? 'Ketik untuk menambah baru' : 'Tidak ada hasil'"
    :reset-filter-on-hide="true"
    @update:model-value="emit('update:modelValue', $event ?? null)"
    @filter="onFilter"
    @show="search(typed)"
    @hide="typed = ''"
  >
    <template #option="{ option }">
      <span v-if="option.isNew">Tambah baru: <b>"{{ option.text }}"</b></span>
      <span v-else>{{ option.text }}</span>
    </template>
  </Select>
</template>
