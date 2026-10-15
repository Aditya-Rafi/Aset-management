<script setup lang="ts">
import Tag from 'primevue/tag'
import { statusLabel, statusSeverity, txTypeLabel, txTypeSeverity } from '@/utils/format'
import type { TxType } from '@/types/models'

const props = defineProps<{ kind: 'asset' | 'tx'; status: string }>()
</script>

<template>
  <Tag
    v-if="props.kind === 'asset'"
    :value="statusLabel(props.status)"
    :severity="statusSeverity(props.status)"
  />
  <Tag v-else :value="txTypeLabel(props.status as TxType)" :severity="txTypeSeverity(props.status as TxType)" />
</template>
