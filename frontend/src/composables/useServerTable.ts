import { onBeforeUnmount, ref, watch } from 'vue'
import { http, qs, type Paged } from '@/api/http'

export interface TableOptions {
  perPage?: number
  /** extra query params, re-read on every load */
  params?: () => Record<string, string | number | undefined | null>
  debounceMs?: number
}

/** State + loader for a PrimeVue DataTable running in lazy (server-side) mode. */
export function useServerTable<T>(endpoint: string, opts: TableOptions = {}) {
  const rows = ref<T[]>([]) as { value: T[] }
  const total = ref(0)
  const loading = ref(false)
  const error = ref('')
  const page = ref(1)
  const perPage = ref(opts.perPage ?? 10)
  const search = ref('')
  const sortField = ref<string | undefined>(undefined)
  const sortOrder = ref<1 | -1 | 0 | null | undefined>(0)

  let seq = 0
  async function load() {
    const mine = ++seq
    loading.value = true
    error.value = ''
    try {
      const res = await http.get<Paged<T>>(
        endpoint +
          qs({
            page: page.value,
            per_page: perPage.value,
            search: search.value,
            sort: sortField.value,
            order: sortOrder.value === -1 ? 'desc' : sortOrder.value === 1 ? 'asc' : undefined,
            ...(opts.params?.() ?? {}),
          }),
      )
      if (mine !== seq) return // a newer request superseded this one
      rows.value = res.data
      total.value = res.meta.total
      // deleting the last row of the last page: step back one page
      if (res.data.length === 0 && page.value > 1 && res.meta.total > 0) {
        page.value = Math.max(1, Math.ceil(res.meta.total / perPage.value))
        await load()
      }
    } catch (e) {
      if (mine !== seq) return
      error.value = e instanceof Error ? e.message : 'Gagal memuat data'
    } finally {
      if (mine === seq) loading.value = false
    }
  }

  function onPage(e: { page: number; rows: number }) {
    page.value = e.page + 1
    perPage.value = e.rows
    load()
  }
  function onSort(e: { sortField?: unknown; sortOrder?: 1 | -1 | 0 | null }) {
    sortField.value = typeof e.sortField === 'string' ? e.sortField : undefined
    sortOrder.value = e.sortOrder
    page.value = 1
    load()
  }

  let timer: ReturnType<typeof setTimeout> | undefined
  watch(search, () => {
    clearTimeout(timer)
    timer = setTimeout(() => {
      page.value = 1
      load()
    }, opts.debounceMs ?? 300)
  })
  onBeforeUnmount(() => clearTimeout(timer))

  /** Reset to page 1 and reload (use after filters change). */
  function refresh() {
    page.value = 1
    return load()
  }

  return { rows, total, loading, error, page, perPage, search, sortField, sortOrder, load, refresh, onPage, onSort }
}
