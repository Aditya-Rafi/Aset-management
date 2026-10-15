<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import Message from 'primevue/message'
import { useLookups } from '@/stores/lookups'
import { assetTitle, txTypeLabel } from '@/utils/format'
import type { Asset, CheckResultValue, Conclusion, InspectionInput, TxType } from '@/types/models'

const props = defineProps<{ visible: boolean; asset: Asset; kind: TxType; modelValue: InspectionInput | null }>()
const emit = defineEmits<{ 'update:visible': [boolean]; save: [InspectionInput] }>()

const lookups = useLookups()
const profile = computed(() => props.asset.category?.inspection ?? 'none')
const tpl = computed(() => lookups.templates[profile.value])

interface Row { result: CheckResultValue; note: string }
const rows = reactive<Record<string, Row>>({})
const acc = reactive<Record<string, { present: boolean | null; condition: string; note: string }>>({})
const battery = ref<number | null>(null)
const conclusion = ref<Conclusion>('')
const conclusionTouched = ref(false)
const notes = ref('')
const skip = ref(false)
const skipReason = ref('')
const error = ref('')

function load() {
  Object.keys(rows).forEach((k) => delete rows[k])
  Object.keys(acc).forEach((k) => delete acc[k])
  const saved = props.modelValue
  for (const s of tpl.value?.sections ?? [])
    for (const it of s.items) {
      const prev = saved?.items?.find((x) => x.code === it.code)
      rows[it.code] = { result: prev?.result ?? '', note: prev?.note ?? '' }
    }
  for (const a of tpl.value?.accessories ?? []) {
    const prev = saved?.accessories?.find((x) => x.code === a.code)
    acc[a.code] = { present: prev?.present ?? null, condition: prev?.condition ?? '', note: prev?.note ?? '' }
  }
  battery.value = saved?.battery_health ?? null
  conclusion.value = saved?.conclusion ?? ''
  conclusionTouched.value = !!saved?.conclusion
  notes.value = saved?.notes ?? ''
  skip.value = !!saved?.skipped
  skipReason.value = saved?.skip_reason ?? ''
  error.value = ''
}

// immediate: the parent mounts this dialog already open
watch(
  () => props.visible,
  async (v) => {
    if (!v) return
    await lookups.loadTemplates()
    load()
  },
  { immediate: true },
)

const badCount = computed(() => Object.values(rows).filter((r) => r.result === 'bad').length)
const answered = computed(() => Object.values(rows).filter((r) => r.result !== '').length)
const total = computed(() => Object.keys(rows).length)

// Suggest a conclusion from the answers until the user picks one themselves.
watch([badCount, answered], () => {
  if (conclusionTouched.value || answered.value === 0) return
  conclusion.value = badCount.value > 0 ? 'perlu_perbaikan' : 'layak'
})

const set = (code: string, v: CheckResultValue) => (rows[code].result = rows[code].result === v ? '' : v)
const allOk = () => Object.values(rows).forEach((r) => (r.result = r.result === 'bad' ? 'bad' : 'ok'))
const pickConclusion = (c: Conclusion) => {
  conclusion.value = c
  conclusionTouched.value = true
}

function submit() {
  error.value = ''
  if (skip.value) {
    if (skipReason.value.trim().length < 3) return void (error.value = 'Tulis alasan melewati pemeriksaan.')
    emit('save', { skipped: true, skip_reason: skipReason.value.trim() })
    emit('update:visible', false)
    return
  }
  if (answered.value === 0) return void (error.value = 'Isi minimal satu butir pemeriksaan, atau lewati dengan alasan.')
  const noNote = Object.entries(rows).find(([, r]) => r.result === 'bad' && !r.note.trim())
  if (noNote) {
    const label = tpl.value?.sections.flatMap((s) => s.items).find((i) => i.code === noNote[0])?.label
    return void (error.value = `Butir Tidak Baik wajib diberi keterangan: ${label}`)
  }
  if (!conclusion.value) return void (error.value = 'Pilih kesimpulan pemeriksaan.')
  emit('save', {
    skipped: false,
    profile: profile.value,
    items: Object.entries(rows).map(([code, r]) => ({ code, result: r.result, note: r.note.trim() })),
    accessories: Object.entries(acc).map(([code, a]) => ({ code, present: a.present, condition: a.condition.trim(), note: a.note.trim() })),
    battery_health: battery.value,
    conclusion: conclusion.value,
    notes: notes.value.trim(),
  })
  emit('update:visible', false)
}
</script>

<template>
  <Dialog
    :visible="visible"
    modal
    :header="`Pemeriksaan fisik · ${asset.tag}`"
    :style="{ width: '56rem', maxWidth: '98vw' }"
    @update:visible="emit('update:visible', $event)"
  >
    <template v-if="tpl">
      <p class="intro">
        <b>{{ assetTitle(asset) }}</b> — {{ txTypeLabel(kind).toLowerCase() }}. Periksa unit di depan karyawan lalu catat kondisinya.
        Hasil ini dibandingkan dengan pemeriksaan berikutnya untuk melihat kapan sebuah komponen mulai rusak.
      </p>

      <div v-if="!skip">
        <div class="bar">
          <b>{{ tpl.title }}</b>
          <span class="muted">{{ answered }}/{{ total }} terisi<span v-if="badCount" class="bad-count"> · {{ badCount }} Tidak Baik</span></span>
          <Button label="Tandai semua Baik" icon="pi pi-check-circle" size="small" severity="secondary" outlined @click="allOk" />
        </div>

        <template v-for="sec in tpl.sections" :key="sec.key">
          <div class="section">{{ sec.title }}</div>
          <div v-for="(it, i) in sec.items" :key="it.code" class="item" :class="{ bad: rows[it.code]?.result === 'bad' }">
            <div class="label">
              <span class="num">{{ i + 1 }}</span>
              <span>{{ it.label }}</span>
              <span v-if="it.battery" class="batt">
                <InputNumber v-model="battery" :min="0" :max="100" suffix=" %" placeholder="health" size="small" input-class="batt-input" :use-grouping="false" />
              </span>
            </div>
            <div class="choices">
              <button type="button" class="chip ok" :class="{ on: rows[it.code]?.result === 'ok' }" @click="set(it.code, 'ok')">Baik</button>
              <button type="button" class="chip no" :class="{ on: rows[it.code]?.result === 'bad' }" @click="set(it.code, 'bad')">Tidak Baik</button>
            </div>
            <InputText
              v-if="rows[it.code]"
              v-model="rows[it.code].note"
              class="note"
              size="small"
              :placeholder="rows[it.code].result === 'bad' ? 'Jelaskan kerusakan (wajib)' : 'Keterangan'"
              :invalid="rows[it.code].result === 'bad' && !rows[it.code].note.trim()"
              maxlength="255"
            />
          </div>
        </template>

        <div class="section">Kelengkapan / Aksesoris</div>
        <div v-for="a in tpl.accessories" :key="a.code" class="item acc">
          <div class="label"><span>{{ a.label }}</span></div>
          <div class="choices">
            <button type="button" class="chip ok" :class="{ on: acc[a.code]?.present === true }" @click="acc[a.code].present = acc[a.code].present === true ? null : true">Ada</button>
            <button type="button" class="chip no" :class="{ on: acc[a.code]?.present === false }" @click="acc[a.code].present = acc[a.code].present === false ? null : false">Tidak</button>
          </div>
          <InputText v-if="acc[a.code]" v-model="acc[a.code].condition" size="small" placeholder="Kondisi" maxlength="100" />
          <InputText v-if="acc[a.code]" v-model="acc[a.code].note" size="small" placeholder="Catatan" maxlength="255" />
        </div>

        <div class="section">Kesimpulan</div>
        <div class="concl">
          <button type="button" class="chip ok" :class="{ on: conclusion === 'layak' }" @click="pickConclusion('layak')">Layak digunakan</button>
          <button type="button" class="chip warn" :class="{ on: conclusion === 'perlu_perbaikan' }" @click="pickConclusion('perlu_perbaikan')">Perlu perbaikan</button>
          <button type="button" class="chip no" :class="{ on: conclusion === 'tidak_layak' }" @click="pickConclusion('tidak_layak')">Tidak layak</button>
        </div>
        <div class="field" style="margin-top: 0.9rem">
          <label for="insp-notes">Catatan kondisi tambahan</label>
          <Textarea id="insp-notes" v-model="notes" rows="2" auto-resize fluid />
        </div>
      </div>

      <div v-else class="field">
        <Message severity="warn" :closable="false">
          Melewati pemeriksaan berarti kondisi unit saat ini <b>tidak tercatat</b>, sehingga kerusakan di kemudian hari sulit dilacak.
        </Message>
        <label for="skip-reason" style="margin-top: 0.75rem">Alasan melewati pemeriksaan *</label>
        <InputText id="skip-reason" v-model="skipReason" maxlength="255" fluid />
      </div>

      <Message v-if="error" severity="error" :closable="false" class="gap">{{ error }}</Message>

      <div class="foot">
        <Button
          :label="skip ? 'Isi pemeriksaan' : 'Lewati pemeriksaan'"
          :icon="skip ? 'pi pi-list-check' : 'pi pi-forward'"
          severity="secondary"
          text
          size="small"
          @click="skip = !skip"
        />
        <span class="grow" />
        <Button label="Batal" severity="secondary" text @click="emit('update:visible', false)" />
        <Button label="Simpan pemeriksaan" icon="pi pi-check" @click="submit" />
      </div>
    </template>
    <p v-else class="muted">Memuat checklist…</p>
  </Dialog>
</template>

<style scoped>
.intro { margin: 0 0 1rem; font-size: 0.92rem; }
.bar { display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap; margin-bottom: 0.5rem; }
.bar .muted { margin-right: auto; font-size: 0.85rem; }
.bad-count { color: var(--p-red-500); font-weight: 600; }
.section { background: var(--p-surface-100); color: var(--p-primary-700); font-weight: 700; font-size: 0.8rem; letter-spacing: 0.04em; padding: 0.4rem 0.6rem; margin-top: 0.9rem; border-radius: 0.35rem; }
.app-dark .section { background: var(--p-surface-800); color: var(--p-primary-300); }
.item { display: grid; grid-template-columns: minmax(0, 1.6fr) max-content minmax(0, 1fr); gap: 0.5rem 0.75rem; align-items: center; padding: 0.4rem 0.2rem; border-bottom: 1px solid var(--p-content-border-color); }
.item.acc { grid-template-columns: minmax(0, 1.2fr) max-content minmax(0, 0.8fr) minmax(0, 1fr); }
.item.bad { background: color-mix(in srgb, var(--p-red-500) 7%, transparent); }
.label { display: flex; align-items: center; gap: 0.5rem; font-size: 0.9rem; flex-wrap: wrap; }
.num { color: var(--p-text-muted-color); width: 1.2rem; flex: none; font-size: 0.8rem; }
.batt { margin-left: auto; }
:deep(.batt-input) { width: 5.5rem; }
.choices, .concl { display: flex; gap: 0.35rem; flex-wrap: wrap; }
.choices { flex-wrap: nowrap; }
.chip { border: 1px solid var(--p-content-border-color); background: var(--p-content-background); color: var(--p-text-color); border-radius: 99px; padding: 0.25rem 0.8rem; font-size: 0.82rem; cursor: pointer; font-family: inherit; white-space: nowrap; flex: none; }
.chip:hover { border-color: var(--p-primary-color); }
.chip.ok.on { background: #16a34a; border-color: #16a34a; color: #fff; }
.chip.no.on { background: #dc2626; border-color: #dc2626; color: #fff; }
.chip.warn.on { background: #d97706; border-color: #d97706; color: #fff; }
.gap { margin: 0.75rem 0; }
.foot { display: flex; gap: 0.5rem; align-items: center; margin-top: 1rem; flex-wrap: wrap; }
.grow { flex: 1; }
@media (max-width: 720px) {
  .item, .item.acc { grid-template-columns: 1fr; }
}
</style>
