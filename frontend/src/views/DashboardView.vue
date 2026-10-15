<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Message from 'primevue/message'
import { http, type Envelope } from '@/api/http'
import {
  LOG_ACTIONS, STATUS, assetTitle, daysUntil, formatDate, formatDateTime, statusLabel,
} from '@/utils/format'
import type { Asset, AssetLog, AssetStatus } from '@/types/models'

interface CategoryRow { id: number; name: string; total: number; available: number; in_use: number; repair: number; broken: number; disposed: number }
interface Stats {
  total: number
  by_status: Record<AssetStatus, number>
  by_category: CategoryRow[]
  employees: number
  employees_holding: number
  warranty_soon: Asset[]
  warranty_expired: number
  attention: Asset[]
  recent_logs: AssetLog[]
}

const router = useRouter()
const stats = ref<Stats | null>(null)
const error = ref('')
const loading = ref(true)

onMounted(async () => {
  try {
    stats.value = (await http.get<Envelope<Stats>>('/dashboard')).data
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Gagal memuat dashboard'
  } finally {
    loading.value = false
  }
})

const cards = computed(() => {
  const s = stats.value
  const n = (k: AssetStatus) => s?.by_status[k] ?? 0
  return [
    { label: 'Total Aset', value: s?.total ?? 0, icon: 'pi-box', tone: 'blue', query: {} },
    { label: 'Tersedia', value: n('available'), icon: 'pi-check-circle', tone: 'green', query: { status: 'available' } },
    { label: 'Dipakai', value: n('in_use'), icon: 'pi-user', tone: 'violet', query: { status: 'in_use' } },
    { label: 'Diservis', value: n('repair'), icon: 'pi-wrench', tone: 'amber', query: { status: 'repair' } },
    { label: 'Rusak', value: n('broken'), icon: 'pi-exclamation-triangle', tone: 'red', query: { status: 'broken' } },
  ]
})

const segments: { key: keyof CategoryRow; status: AssetStatus }[] = [
  { key: 'available', status: 'available' }, { key: 'in_use', status: 'in_use' },
  { key: 'repair', status: 'repair' }, { key: 'broken', status: 'broken' }, { key: 'disposed', status: 'disposed' },
]
const maxTotal = computed(() => Math.max(1, ...(stats.value?.by_category.map((c) => c.total) ?? [1])))
</script>

<template>
  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

  <div class="cards">
    <button v-for="c in cards" :key="c.label" class="stat page-card" :class="c.tone" @click="router.push({ name: 'asset', query: c.query })">
      <i class="pi" :class="c.icon" />
      <div>
        <div class="muted label">{{ c.label }}</div>
        <div class="value">{{ loading ? '…' : c.value.toLocaleString('id-ID') }}</div>
      </div>
    </button>
  </div>

  <div class="two">
    <div class="page-card">
      <h3>Aset per Kategori</h3>
      <p v-if="!loading && !stats?.by_category.some((c) => c.total)" class="muted">Belum ada aset. Tambahkan lewat menu Daftar Aset atau Impor CSV.</p>
      <div class="legend">
        <span v-for="s in segments" :key="s.status"><i class="sw" :class="s.status" /> {{ statusLabel(s.status) }}</span>
      </div>
      <template v-for="c in stats?.by_category" :key="c.id">
        <div v-if="c.total" class="bar-row clickable" @click="router.push({ name: 'asset', query: { category_id: c.id } })">
          <span class="name">{{ c.name }}</span>
          <div class="track" :style="{ width: (c.total / maxTotal) * 100 + '%' }">
            <div v-for="s in segments" :key="s.status" class="seg" :class="s.status" :style="{ flex: c[s.key] }" :title="`${statusLabel(s.status)}: ${c[s.key]}`" />
          </div>
          <span class="count mono">{{ c.total }}</span>
        </div>
      </template>
    </div>

    <div class="stack">
      <div class="page-card">
        <h3>Perlu Perhatian <span class="muted">({{ (stats?.by_status.repair ?? 0) + (stats?.by_status.broken ?? 0) }})</span></h3>
        <p v-if="!loading && !stats?.attention.length" class="muted">Tidak ada aset rusak atau sedang diservis.</p>
        <ul class="list">
          <li v-for="a in stats?.attention" :key="a.id">
            <RouterLink :to="{ name: 'asset.detail', params: { tag: a.tag } }" class="link mono">{{ a.tag }}</RouterLink>
            <span>{{ assetTitle(a) }}</span>
            <span class="pill" :class="a.status">{{ statusLabel(a.status) }}</span>
          </li>
        </ul>
      </div>

      <div class="page-card">
        <h3>
          Garansi Segera Habis
          <span v-if="stats?.warranty_expired" class="muted">· {{ stats.warranty_expired }} sudah habis</span>
        </h3>
        <p v-if="!loading && !stats?.warranty_soon.length" class="muted">Tidak ada garansi yang habis dalam 60 hari.</p>
        <ul class="list">
          <li v-for="a in stats?.warranty_soon" :key="a.id">
            <RouterLink :to="{ name: 'asset.detail', params: { tag: a.tag } }" class="link mono">{{ a.tag }}</RouterLink>
            <span>{{ assetTitle(a) }}</span>
            <span class="muted nowrap">{{ formatDate(a.warranty_until) }} ({{ daysUntil(a.warranty_until) }} hari)</span>
          </li>
        </ul>
      </div>
    </div>
  </div>

  <div class="page-card gap">
    <h3>Aktivitas Terbaru <RouterLink :to="{ name: 'activity' }" class="link more">Lihat semua</RouterLink></h3>
    <p v-if="!loading && !stats?.recent_logs.length" class="muted">Belum ada aktivitas.</p>
    <ul class="list">
      <li v-for="l in stats?.recent_logs" :key="l.id">
        <i class="pi act" :class="LOG_ACTIONS[l.action]?.icon" />
        <RouterLink v-if="l.asset" :to="{ name: 'asset.detail', params: { tag: l.asset.tag } }" class="link mono">{{ l.asset.tag }}</RouterLink>
        <span class="grow">{{ LOG_ACTIONS[l.action]?.label }}<span v-if="l.message" class="muted"> — {{ l.message }}</span></span>
        <span class="muted nowrap small">{{ formatDateTime(l.created_at) }}</span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr)); gap: 1rem; margin-bottom: 1rem; }
.stat { display: flex; align-items: center; gap: 1rem; text-align: left; cursor: pointer; font: inherit; color: inherit; transition: box-shadow 0.15s; }
.stat:hover { box-shadow: 0 2px 12px rgb(0 0 0 / 0.1); }
.stat .pi { font-size: 1.3rem; width: 2.8rem; height: 2.8rem; display: grid; place-items: center; border-radius: 0.8rem; flex: none; }
.label { font-size: 0.85rem; }
.value { font-size: 1.6rem; font-weight: 700; }
.blue .pi { background: #dbeafe; color: #1d4ed8; } .green .pi { background: #dcfce7; color: #15803d; }
.red .pi { background: #fee2e2; color: #b91c1c; } .violet .pi { background: #ede9fe; color: #6d28d9; }
.amber .pi { background: #fef3c7; color: #b45309; }
.two { display: grid; grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr); gap: 1rem; margin-bottom: 1rem; align-items: start; }
@media (max-width: 1000px) { .two { grid-template-columns: 1fr; } }
.stack { display: grid; gap: 1rem; }
.gap { margin-top: 0; }
h3 { margin: 0 0 1rem; font-size: 1rem; }
.more { font-size: 0.8rem; margin-left: 0.5rem; font-weight: 500; }
.legend { display: flex; flex-wrap: wrap; gap: 0.25rem 1rem; font-size: 0.8rem; margin-bottom: 0.9rem; color: var(--p-text-muted-color); }
.sw { display: inline-block; width: 0.7rem; height: 0.7rem; border-radius: 3px; margin-right: 0.25rem; }
.bar-row { display: grid; grid-template-columns: 8.5rem 1fr 2.5rem; gap: 0.75rem; align-items: center; margin-bottom: 0.55rem; font-size: 0.9rem; }
.bar-row .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.track { display: flex; height: 0.75rem; border-radius: 99px; overflow: hidden; min-width: 0.75rem; }
.count { text-align: right; }
.seg, .sw { background: var(--c); }
.available { --c: #22c55e; } .in_use { --c: #6366f1; } .repair { --c: #f59e0b; } .broken { --c: #ef4444; } .disposed { --c: #9ca3af; }
.list { list-style: none; margin: 0; padding: 0; display: grid; gap: 0.6rem; font-size: 0.9rem; }
.list li { display: flex; gap: 0.6rem; align-items: baseline; flex-wrap: wrap; }
.grow { flex: 1; min-width: 8rem; }
.pill { font-size: 0.72rem; padding: 0.1rem 0.5rem; border-radius: 99px; color: #fff; background: var(--c); margin-left: auto; }
.act { color: var(--p-primary-color); font-size: 0.85rem; }
.small { font-size: 0.78rem; }
</style>
