<script setup lang="ts">
import { ref } from 'vue'
import AutoComplete from 'primevue/autocomplete'
import { http, qs } from '@/api/http'

const props = defineProps<{ modelValue: string | null | undefined; field: string; invalid?: boolean; id?: string; placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const suggestions = ref<string[]>([])
let seq = 0

// Suggest what was typed before (brands, locations…) so spelling stays consistent.
async function complete(e: { query: string }) {
  const mine = ++seq
  try {
    const res = await http.get<string[]>('/suggest' + qs({ field: props.field, search: e.query }))
    if (mine === seq) suggestions.value = res
  } catch {
    if (mine === seq) suggestions.value = []
  }
}
</script>

<template>
  <AutoComplete
    :model-value="modelValue ?? ''"
    :suggestions="suggestions"
    :input-id="id"
    :invalid="invalid"
    :placeholder="placeholder"
    :min-length="0"
    fluid
    @complete="complete"
    @update:model-value="emit('update:modelValue', String($event ?? ''))"
  />
</template>
