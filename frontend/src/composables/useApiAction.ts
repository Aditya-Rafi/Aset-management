import { useToast } from 'primevue/usetoast'
import { ApiError } from '@/api/http'

/** Shows API failures as toasts; returns field errors for inline display. */
export function useNotify() {
  const toast = useToast()
  return {
    success: (detail: string) => toast.add({ severity: 'success', summary: 'Berhasil', detail, life: 3500 }),
    error: (detail: string) => toast.add({ severity: 'error', summary: 'Gagal', detail, life: 6000 }),
    fromError(e: unknown): Record<string, string> {
      if (e instanceof ApiError) {
        if (Object.keys(e.errors).length === 0) this.error(e.message)
        return e.errors
      }
      this.error('Terjadi kesalahan tak terduga')
      return {}
    },
  }
}
