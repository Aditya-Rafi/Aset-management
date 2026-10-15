<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import { http, ApiError } from '@/api/http'
import { tagFromScan } from '@/utils/format'

const router = useRouter()

const code = ref('')
const error = ref('')
const looking = ref(false)

async function open(raw: string) {
  const tag = tagFromScan(raw)
  if (!tag || looking.value) return
  looking.value = true
  error.value = ''
  try {
    await http.get(`/assets/tag/${encodeURIComponent(tag)}`)
    stop()
    router.push({ name: 'asset.detail', params: { tag } })
  } catch (e) {
    error.value = e instanceof ApiError && e.status === 404 ? `Kode "${tag}" tidak dikenal.` : 'Gagal mencari aset.'
  } finally {
    looking.value = false
  }
}

// ---- camera (BarcodeDetector is built into Chrome/Edge on Android and desktop) ----
interface Detector { detect(src: CanvasImageSource): Promise<{ rawValue: string }[]> }
const supported = typeof window !== 'undefined' && 'BarcodeDetector' in window
const insecure = typeof window !== 'undefined' && !window.isSecureContext
const video = ref<HTMLVideoElement | null>(null)
const running = ref(false)
const cameraError = ref('')
let stream: MediaStream | null = null
let timer: ReturnType<typeof setInterval> | undefined
let lastValue = ''

async function start() {
  cameraError.value = ''
  try {
    const Ctor = (window as unknown as { BarcodeDetector: new (o: { formats: string[] }) => Detector }).BarcodeDetector
    const detector = new Ctor({ formats: ['qr_code', 'code_128'] })
    stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: 'environment' } }, audio: false })
    running.value = true
    await Promise.resolve()
    if (!video.value) return
    video.value.srcObject = stream
    await video.value.play()
    timer = setInterval(async () => {
      if (!video.value || video.value.readyState < 2) return
      try {
        const found = await detector.detect(video.value)
        const v = found[0]?.rawValue
        if (v && v !== lastValue) {
          lastValue = v
          await open(v)
        }
      } catch {
        /* a frame failed to decode, try the next one */
      }
    }, 250)
  } catch (e) {
    stop()
    cameraError.value =
      e instanceof DOMException && e.name === 'NotAllowedError'
        ? 'Izin kamera ditolak. Izinkan kamera untuk situs ini di pengaturan browser.'
        : 'Kamera tidak dapat dibuka.'
  }
}

function stop() {
  clearInterval(timer)
  stream?.getTracks().forEach((t) => t.stop())
  stream = null
  running.value = false
  lastValue = ''
}
onBeforeUnmount(stop)
</script>

<template>
  <div class="page-card narrow">
    <div class="page-head"><h2>Scan Kode Aset</h2></div>

    <div class="field">
      <label for="code"><i class="pi pi-qrcode" /> Ketik kode, atau scan dengan scanner barcode, lalu Enter</label>
      <InputText id="code" v-model="code" placeholder="LPT-0001" autofocus fluid :disabled="looking" @keydown.enter.prevent="open(code)" />
    </div>
    <Message v-if="error" severity="error" :closable="false" class="gap">{{ error }}</Message>
    <Button label="Buka" icon="pi pi-arrow-right" :loading="looking" :disabled="!code.trim()" @click="open(code)" />

    <hr />

    <h3>Scan dengan kamera</h3>
    <Message v-if="!supported" severity="warn" :closable="false">
      Browser ini belum mendukung scan kamera di dalam aplikasi. Anda tetap bisa memindai stiker dengan <b>aplikasi kamera bawaan HP</b>
      atau Google Lens — QR di stiker membuka halaman aset langsung.
    </Message>
    <Message v-else-if="insecure" severity="warn" :closable="false">
      Kamera di dalam browser hanya aktif lewat HTTPS (atau localhost). Pakai aplikasi kamera bawaan HP untuk memindai QR stiker, atau akses aplikasi ini lewat HTTPS.
    </Message>
    <template v-else>
      <Message v-if="cameraError" severity="error" :closable="false" class="gap">{{ cameraError }}</Message>
      <div v-show="running" class="viewport">
        <video ref="video" playsinline muted />
        <div class="frame" />
      </div>
      <Button v-if="!running" label="Nyalakan kamera" icon="pi pi-camera" severity="secondary" @click="start" />
      <Button v-else label="Matikan kamera" icon="pi pi-stop-circle" severity="secondary" outlined @click="stop" />
    </template>
  </div>
</template>

<style scoped>
.narrow { max-width: 34rem; margin: 0 auto; }
.gap { margin: 0.75rem 0; }
hr { border: 0; border-top: 1px solid var(--p-content-border-color); margin: 1.5rem 0; }
h3 { margin: 0 0 0.75rem; font-size: 1rem; }
.viewport { position: relative; border-radius: 0.75rem; overflow: hidden; background: #000; margin-bottom: 0.75rem; aspect-ratio: 4 / 3; }
.viewport video { width: 100%; height: 100%; object-fit: cover; }
.frame { position: absolute; inset: 18%; border: 3px solid rgb(255 255 255 / 0.85); border-radius: 0.75rem; box-shadow: 0 0 0 999px rgb(0 0 0 / 0.35); pointer-events: none; }
</style>
