export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public errors: Record<string, string> = {},
  ) {
    super(message)
  }
}

type Unauthorized = () => void
let onUnauthorized: Unauthorized = () => {}
export const setUnauthorizedHandler = (fn: Unauthorized) => (onUnauthorized = fn)

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, credentials: 'same-origin', headers: {} }
  if (body instanceof FormData) {
    init.body = body
  } else if (body !== undefined) {
    ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }

  let res: Response
  try {
    res = await fetch('/api' + url, init)
  } catch {
    throw new ApiError(0, 'Tidak dapat terhubung ke server')
  }

  const isJson = res.headers.get('content-type')?.includes('application/json')
  const payload = isJson ? await res.json() : null

  if (!res.ok) {
    if (res.status === 401 && !url.startsWith('/auth/login')) onUnauthorized()
    throw new ApiError(res.status, payload?.message ?? `Permintaan gagal (${res.status})`, payload?.errors ?? {})
  }
  return payload as T
}

export const http = {
  get: <T>(url: string) => request<T>('GET', url),
  post: <T>(url: string, body?: unknown) => request<T>('POST', url, body),
  put: <T>(url: string, body?: unknown) => request<T>('PUT', url, body),
  delete: <T>(url: string) => request<T>('DELETE', url),
}

export interface Envelope<T> {
  success: boolean
  message?: string
  data: T
}
export interface Paged<T> {
  success: boolean
  data: T[]
  meta: { total: number; page: number; per_page: number }
}

export function qs(params: Record<string, string | number | undefined | null>): string {
  const u = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') u.set(k, String(v))
  const s = u.toString()
  return s ? '?' + s : ''
}
