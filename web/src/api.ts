export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

function cookieValue(name: string) {
	if (typeof document === 'undefined') return ''
	const prefix = `${encodeURIComponent(name)}=`
	const cookie = document.cookie.split('; ').find((entry) => entry.startsWith(prefix))
	return cookie ? decodeURIComponent(cookie.slice(prefix.length)) : ''
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
	const headers = new Headers(init.headers)
	const isFormData = typeof FormData !== 'undefined' && init.body instanceof FormData
	const method = (init.method ?? 'GET').toUpperCase()
	if (!isFormData && !headers.has('Content-Type')) {
		headers.set('Content-Type', 'application/json')
	}
	if (!['GET', 'HEAD', 'OPTIONS'].includes(method) && !headers.has('X-CSRF-Token')) {
		const csrfToken = cookieValue('app_csrf')
		if (csrfToken) headers.set('X-CSRF-Token', csrfToken)
	}
  const response = await fetch(path, {
		...init,
    credentials: 'include',
		headers,
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

export function postForm<T>(path: string, body: FormData) {
	return api<T>(path, { method: 'POST', body })
}
