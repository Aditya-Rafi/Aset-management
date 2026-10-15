import type { Asset, AssetStatus, TxType } from '@/types/models'

const dtf = new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short' })
const df = new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium' })
const nf = new Intl.NumberFormat('id-ID')

export const formatDateTime = (v?: string | null) => (v ? dtf.format(new Date(v)) : '-')
export const formatDate = (v?: string | null) => (v ? df.format(new Date(v.length === 10 ? v + 'T00:00:00' : v)) : '-')
export const formatRupiah = (v?: number | null) => (v === null || v === undefined ? '-' : 'Rp ' + nf.format(v))
export const dash = (v?: string | number | null) => (v === null || v === undefined || v === '' ? '-' : String(v))

type Severity = 'success' | 'info' | 'warn' | 'danger' | 'secondary'
export const STATUS: Record<AssetStatus, { label: string; severity: Severity }> = {
  available: { label: 'Tersedia', severity: 'success' },
  in_use: { label: 'Dipakai', severity: 'info' },
  repair: { label: 'Diservis', severity: 'warn' },
  broken: { label: 'Rusak', severity: 'danger' },
  disposed: { label: 'Dimusnahkan', severity: 'secondary' },
}
export const statusLabel = (s: string) => STATUS[s as AssetStatus]?.label ?? s
export const statusSeverity = (s: string): Severity => STATUS[s as AssetStatus]?.severity ?? 'secondary'
export const statusOptions = (Object.keys(STATUS) as AssetStatus[]).map((v) => ({ value: v, label: STATUS[v].label }))

export const txTypeLabel = (t: TxType) => ({ out: 'Serah Terima', in: 'Pengembalian', transfer: 'Pindah Tangan' })[t] ?? t
export const txTypeSeverity = (t: TxType): Severity => ({ out: 'info', in: 'success', transfer: 'warn' } as Record<string, Severity>)[t] ?? 'secondary'

export const CONCLUSION: Record<string, { label: string; severity: Severity }> = {
  layak: { label: 'Layak digunakan', severity: 'success' },
  perlu_perbaikan: { label: 'Perlu perbaikan', severity: 'warn' },
  tidak_layak: { label: 'Tidak layak', severity: 'danger' },
}
export const INSPECTION_PROFILES = [
  { value: 'none', label: 'Tanpa pemeriksaan' },
  { value: 'general', label: 'Pemeriksaan umum (fisik & fungsi)' },
  { value: 'laptop', label: 'Pemeriksaan laptop (hardware & software)' },
]

export const LOG_ACTIONS: Record<string, { label: string; icon: string }> = {
  created: { label: 'Didaftarkan', icon: 'pi-plus-circle' },
  updated: { label: 'Diubah', icon: 'pi-pencil' },
  status: { label: 'Status', icon: 'pi-flag' },
  assigned: { label: 'Diserahkan', icon: 'pi-arrow-right' },
  returned: { label: 'Dikembalikan', icon: 'pi-arrow-left' },
  note: { label: 'Catatan', icon: 'pi-comment' },
  photo: { label: 'Foto', icon: 'pi-image' },
  reverted: { label: 'Dibatalkan', icon: 'pi-undo' },
  transferred: { label: 'Pindah tangan', icon: 'pi-sync' },
}

export const SPEC_LABELS: Record<string, string> = {
  cpu: 'CPU', ram: 'RAM', storage: 'Storage', os: 'OS', screen_size: 'Ukuran Layar',
}

/** "Lenovo IdeaPad Slim 3" — falls back to the category name for unbranded items. */
export function assetTitle(a: Pick<Asset, 'brand' | 'model' | 'category'>): string {
  const t = [a.brand, a.model].filter(Boolean).join(' ').trim()
  return t || a.category?.name || '-'
}

/** One-line hardware summary: "Intel i5 · 16 GB · 512 GB · Windows 11". */
export function specSummary(a: Asset): string {
  return [a.cpu, a.ram, a.storage, a.os, a.screen_size].filter(Boolean).join(' · ')
}

/** Days until a YYYY-MM-DD date (negative when past). */
export function daysUntil(date?: string | null): number | null {
  if (!date) return null
  const ms = new Date(date + 'T00:00:00').getTime() - new Date(new Date().toDateString()).getTime()
  return Math.round(ms / 86400000)
}

/** The tag inside a scanned value: a sticker URL (…/a/LPT-0001) or the bare tag. */
export function tagFromScan(raw: string): string {
  const v = raw.trim()
  try {
    const u = new URL(v)
    const m = u.pathname.match(/\/a\/([^/]+)/)
    if (m) return decodeURIComponent(m[1]).toUpperCase()
  } catch {
    /* not a URL */
  }
  return v.toUpperCase()
}
