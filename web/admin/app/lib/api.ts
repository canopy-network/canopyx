// Minimal fetch wrapper for our admin API:
// - reads base from NEXT_PUBLIC_API_BASE (used in dev)
// - always sends cookies (credentials: 'include')
//
// In static export (served by Go), keep NEXT_PUBLIC_API_BASE empty so
// all calls are relative (same-origin).

export const API_BASE =
    (typeof process !== 'undefined' && process.env.NEXT_PUBLIC_API_BASE) || ''

export function apiFetch(path: string, init: RequestInit = {}) {
    if (!init.headers) init.headers = {}

    init.headers['Content-Type'] = 'application/json'
    init.headers['X-Session'] = '1'

    const url = `${API_BASE}${path}`
    return fetch(url, {
        credentials: 'include',
        ...init,
    })
}
