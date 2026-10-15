<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import { http, ApiError, type Envelope } from '@/api/http'
import { useLookups } from '@/stores/lookups'
import { useNotify } from '@/composables/useApiAction'
import AutocompleteInput from '@/components/AutocompleteInput.vue'
import { SPEC_LABELS, statusOptions } from '@/utils/format'
import type { Asset } from '@/types/models'

const MAX_IMAGES = 5
const MAX_BYTES = 2 * 1024 * 1024

const props = defineProps<{ visible: boolean; asset: Asset | null }>()
const emit = defineEmits<{ 'update:visible': [boolean]; saved: [Asset] }>()

const lookups = useLookups()
const notify = useNotify()

const blank = () => ({
  category_id: null as number | null, brand: '', model: '', serial_number: '',
  cpu: '', ram: '', storage: '', os: '', screen_size: '',
  manufacture_year: '', purchase_date: '', purchase_price: '', warranty_until: '',
  vendor: '', location: '', quantity: '1', notes: '', status: 'available',
})
const form = reactive(blank())
const errors = ref<Record<string, string>>({})
const saving = ref(false)
const photos = ref<{ file: File; url: string }[]>([])

const isEdit = computed(() => !!props.asset)
const category = computed(() => lookups.byId(form.category_id))
const specFields = computed(() => category.value?.spec_fields ?? [])
const createStatuses = statusOptions.filter((s) => s.value !== 'in_use')

function clearPhotos() {
  photos.value.forEach((p) => URL.revokeObjectURL(p.url))
  photos.value = []
}
onBeforeUnmount(clearPhotos)

watch(
  () => props.visible,
  async (v) => {
    if (!v) return
    await lookups.loadCategories()
    errors.value = {}
    clearPhotos()
    const a = props.asset
    Object.assign(form, blank())
    if (a) {
      Object.assign(form, {
        category_id: a.category_id, brand: a.brand ?? '', model: a.model ?? '', serial_number: a.serial_number ?? '',
        cpu: a.cpu ?? '', ram: a.ram ?? '', storage: a.storage ?? '', os: a.os ?? '', screen_size: a.screen_size ?? '',
        manufacture_year: a.manufacture_year?.toString() ?? '', purchase_date: a.purchase_date ?? '',
        purchase_price: a.purchase_price?.toString() ?? '', warranty_until: a.warranty_until ?? '',
        vendor: a.vendor ?? '', location: a.location ?? '', quantity: String(a.quantity), notes: a.notes ?? '',
      })
    }
  },
)

function onFiles(e: Event) {
  const input = e.target as HTMLInputElement
  const problems: string[] = []
  for (const file of Array.from(input.files ?? [])) {
    if (photos.value.length >= MAX_IMAGES) {
      problems.push(`Maksimal ${MAX_IMAGES} foto`)
      break
    }
    if (!['image/jpeg', 'image/png', 'image/gif'].includes(file.type)) problems.push(`${file.name}: harus jpeg, png, atau gif`)
    else if (file.size > MAX_BYTES) problems.push(`${file.name}: maksimal 2MB`)
    else photos.value.push({ file, url: URL.createObjectURL(file) })
  }
  input.value = ''
  errors.value = { ...errors.value, images: problems.join('; ') }
  if (!problems.length) delete errors.value.images
}

function dropPhoto(i: number) {
  URL.revokeObjectURL(photos.value[i].url)
  photos.value.splice(i, 1)
}

const num = (s: string) => (s.trim() === '' ? null : Number(s))

function payload() {
  const b: Record<string, unknown> = {
    category_id: form.category_id ?? 0,
    brand: form.brand, model: form.model, serial_number: form.serial_number,
    cpu: form.cpu, ram: form.ram, storage: form.storage, os: form.os, screen_size: form.screen_size,
    manufacture_year: num(form.manufacture_year), purchase_date: form.purchase_date || null,
    purchase_price: num(form.purchase_price), warranty_until: form.warranty_until || null,
    vendor: form.vendor, location: form.location, quantity: num(form.quantity) ?? 1, notes: form.notes,
  }
  if (!isEdit.value) b.status = form.status
  return b
}

async function save() {
  saving.value = true
  errors.value = {}
  try {
    const res = props.asset
      ? await http.put<Envelope<Asset>>(`/assets/${props.asset.id}`, payload())
      : await http.post<Envelope<Asset>>('/assets', payload())
    let saved = res.data
    if (!props.asset && photos.value.length) {
      const fd = new FormData()
      photos.value.forEach((p) => fd.append('images', p.file))
      try {
        saved = (await http.post<Envelope<Asset>>(`/assets/${saved.id}/images`, fd)).data
      } catch (e) {
        notify.error('Aset tersimpan, tetapi foto gagal diunggah: ' + (e instanceof ApiError ? e.message : 'kesalahan jaringan'))
      }
    }
    notify.success(isEdit.value ? `Aset ${saved.tag} diperbarui` : `Aset ${saved.tag} ditambahkan`)
    emit('saved', saved)
    emit('update:visible', false)
  } catch (e) {
    errors.value = notify.fromError(e)
  } finally {
    saving.value = false
  }
}

const err = (k: string) => errors.value[k]
</script>

<template>
  <Dialog
    :visible="visible"
    modal
    :header="isEdit ? `Ubah Aset ${asset?.tag}` : 'Tambah Aset'"
    :style="{ width: '48rem', maxWidth: '96vw' }"
    @update:visible="emit('update:visible', $event)"
  >
    <form novalidate @submit.prevent="save">
      <div class="grid-2">
        <div class="field">
          <label>Kategori *</label>
          <Select
            v-model="form.category_id"
            :options="lookups.categories"
            option-label="name"
            option-value="id"
            placeholder="Pilih kategori"
            filter
            :invalid="!!err('category_id')"
            fluid
          />
          <small v-if="err('category_id')" class="field-error">{{ err('category_id') }}</small>
          <small v-else-if="category && !isEdit" class="muted">Kode aset akan berawalan {{ category.code }}-</small>
        </div>
        <div class="field">
          <label for="brand">Merk</label>
          <AutocompleteInput id="brand" v-model="form.brand" field="brand" :invalid="!!err('brand')" />
          <small v-if="err('brand')" class="field-error">{{ err('brand') }}</small>
        </div>
        <div class="field">
          <label for="model">Tipe / Model</label>
          <AutocompleteInput id="model" v-model="form.model" field="model" :invalid="!!err('model')" />
          <small v-if="err('model')" class="field-error">{{ err('model') }}</small>
        </div>
        <div class="field">
          <label for="sn">Serial Number</label>
          <InputText id="sn" v-model="form.serial_number" :invalid="!!err('serial_number')" fluid />
          <small v-if="err('serial_number')" class="field-error">{{ err('serial_number') }}</small>
        </div>
      </div>

      <div v-if="specFields.length" class="grid-2">
        <div v-for="f in specFields" :key="f" class="field">
          <label :for="'s-' + f">{{ SPEC_LABELS[f] }}</label>
          <AutocompleteInput :id="'s-' + f" v-model="(form as Record<string, any>)[f]" :field="f" :invalid="!!err(f)" />
          <small v-if="err(f)" class="field-error">{{ err(f) }}</small>
        </div>
      </div>

      <div class="grid-3">
        <div class="field">
          <label for="year">Tahun Pembuatan</label>
          <InputText id="year" v-model="form.manufacture_year" type="number" min="1980" :invalid="!!err('manufacture_year')" fluid />
          <small v-if="err('manufacture_year')" class="field-error">{{ err('manufacture_year') }}</small>
        </div>
        <div class="field">
          <label for="pdate">Tanggal Pembelian</label>
          <InputText id="pdate" v-model="form.purchase_date" type="date" :invalid="!!err('purchase_date')" fluid />
        </div>
        <div class="field">
          <label for="price">Harga Beli (Rp)</label>
          <InputText id="price" v-model="form.purchase_price" type="number" min="0" :invalid="!!err('purchase_price')" fluid />
          <small v-if="err('purchase_price')" class="field-error">{{ err('purchase_price') }}</small>
        </div>
        <div class="field">
          <label for="warranty">Garansi Sampai</label>
          <InputText id="warranty" v-model="form.warranty_until" type="date" fluid />
        </div>
        <div class="field">
          <label for="vendor">Vendor / Toko</label>
          <AutocompleteInput id="vendor" v-model="form.vendor" field="vendor" />
        </div>
        <div class="field">
          <label for="qty">Jumlah</label>
          <InputText id="qty" v-model="form.quantity" type="number" min="1" :invalid="!!err('quantity')" fluid />
          <small v-if="err('quantity')" class="field-error">{{ err('quantity') }}</small>
        </div>
      </div>

      <div class="grid-2">
        <div class="field">
          <label for="loc">Lokasi</label>
          <AutocompleteInput id="loc" v-model="form.location" field="location" placeholder="mis. Ruang Server, Lemari IT" />
          <small class="muted">Untuk aset yang sedang dipakai, lokasinya otomatis mengikuti pemegang.</small>
        </div>
        <div v-if="!isEdit" class="field">
          <label>Status awal</label>
          <Select v-model="form.status" :options="createStatuses" option-label="label" option-value="value" :invalid="!!err('status')" fluid />
          <small v-if="err('status')" class="field-error">{{ err('status') }}</small>
          <small v-else class="muted">Untuk status Dipakai, buat dulu lalu lakukan Serah Terima.</small>
        </div>
      </div>

      <div class="field">
        <label for="notes">Keterangan</label>
        <Textarea id="notes" v-model="form.notes" rows="3" auto-resize fluid />
        <small class="muted">Jangan simpan password atau data rahasia di sini.</small>
      </div>

      <div v-if="!isEdit" class="field">
        <label>Foto ({{ photos.length }}/{{ MAX_IMAGES }}) — jpeg/png/gif, maks. 2MB</label>
        <div class="photos">
          <div v-for="(p, i) in photos" :key="p.url" class="photo">
            <img :src="p.url" alt="" />
            <button type="button" aria-label="Hapus foto" @click="dropPhoto(i)"><i class="pi pi-times" /></button>
          </div>
          <label v-if="photos.length < MAX_IMAGES" class="photo add" tabindex="0">
            <i class="pi pi-camera" />
            <input type="file" accept="image/jpeg,image/png,image/gif" multiple capture="environment" hidden @change="onFiles" />
          </label>
        </div>
        <small v-if="err('images')" class="field-error">{{ err('images') }}</small>
      </div>

      <div class="toolbar" style="justify-content: flex-end">
        <Button type="button" label="Batal" severity="secondary" text @click="emit('update:visible', false)" />
        <Button type="submit" label="Simpan" icon="pi pi-check" :loading="saving" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.photos { display: flex; flex-wrap: wrap; gap: 0.6rem; }
.photo { position: relative; width: 5rem; height: 5rem; border-radius: 0.5rem; overflow: hidden; border: 1px solid var(--p-content-border-color); background: var(--p-surface-100); display: grid; place-items: center; }
.photo img { width: 100%; height: 100%; object-fit: cover; }
.photo button { position: absolute; top: 0.2rem; right: 0.2rem; width: 1.4rem; height: 1.4rem; border-radius: 50%; border: 0; background: rgb(0 0 0 / 0.65); color: #fff; cursor: pointer; display: grid; place-items: center; font-size: 0.7rem; }
.photo.add { cursor: pointer; border-style: dashed; color: var(--p-text-muted-color); }
.photo.add:hover { border-color: var(--p-primary-color); color: var(--p-primary-color); }
</style>
