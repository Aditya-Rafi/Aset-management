import { describe, expect, it } from 'vitest'
import { assetTitle, dash, daysUntil, formatDate, formatRupiah, specSummary, statusLabel, tagFromScan, txTypeLabel } from './format'
import type { Asset } from '@/types/models'

describe('format', () => {
  it('formats rupiah and empty values', () => {
    expect(formatRupiah(2500000)).toBe('Rp 2.500.000')
    expect(formatRupiah(null)).toBe('-')
    expect(dash('')).toBe('-')
    expect(dash(0)).toBe('0')
  })
  it('labels statuses and document types in Indonesian', () => {
    expect(statusLabel('available')).toBe('Tersedia')
    expect(statusLabel('in_use')).toBe('Dipakai')
    expect(statusLabel('repair')).toBe('Diservis')
    expect(statusLabel('broken')).toBe('Rusak')
    expect(statusLabel('disposed')).toBe('Dimusnahkan')
    expect(txTypeLabel('out')).toBe('Serah Terima')
    expect(txTypeLabel('in')).toBe('Pengembalian')
    expect(txTypeLabel('transfer')).toBe('Pindah Tangan')
  })
  it('does not shift plain dates across timezones', () => {
    expect(formatDate('2024-01-31')).toContain('31')
  })
  it('builds titles and spec summaries', () => {
    const a = { brand: 'Lenovo', model: 'IdeaPad', cpu: 'i5', ram: '16 GB', storage: null, os: 'Windows 11', screen_size: null } as Asset
    expect(assetTitle(a)).toBe('Lenovo IdeaPad')
    expect(specSummary(a)).toBe('i5 · 16 GB · Windows 11')
    expect(assetTitle({ brand: null, model: null, category: { name: 'Kabel' } } as Asset)).toBe('Kabel')
  })
  it('counts days until a date', () => {
    const d = new Date()
    d.setDate(d.getDate() + 10)
    const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    expect(daysUntil(iso)).toBe(10)
    expect(daysUntil(null)).toBeNull()
  })
  it('extracts the asset tag from whatever a scanner returns', () => {
    expect(tagFromScan('http://192.168.1.10:8090/a/LPT-0001')).toBe('LPT-0001')
    expect(tagFromScan('https://aset.kantor.id/a/mon-0004?x=1')).toBe('MON-0004')
    expect(tagFromScan('  lpt-0002 ')).toBe('LPT-0002')
  })
})
