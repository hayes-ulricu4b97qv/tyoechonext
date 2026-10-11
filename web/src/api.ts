export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers ?? {}),
    },
    ...init,
  })
  if (response.status === 204) return undefined as T
  const payload = await response.json().catch(() => ({}))
  if (!response.ok) {
    const error = payload?.error
    throw new ApiError(response.status, error?.code ?? 'request_failed', error?.message ?? '请求失败')
  }
  return payload as T
}

export function postJSON<T>(path: string, body: unknown) {
  return api<T>(path, { method: 'POST', body: JSON.stringify(body) })
}

export function putJSON<T>(path: string, body: unknown) {
  return api<T>(path, { method: 'PUT', body: JSON.stringify(body) })
}
