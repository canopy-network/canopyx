// Minimal fetch wrapper for our admin API:
// - reads base from NEXT_PUBLIC_API_BASE (used in dev)
// - always sends cookies (credentials: 'include')
//
// In static export (served by Go), keep NEXT_PUBLIC_API_BASE empty so
// all calls are relative (same-origin).

export const API_BASE =
    (typeof process !== 'undefined' && process.env.NEXT_PUBLIC_API_BASE) || ''

export async function apiFetch(path: string, init: RequestInit = {}) {
    const headers = new Headers(init.headers || {})
    if (!headers.has('Content-Type')) headers.set('Content-Type', 'application/json')

    const res = await fetch(`${API_BASE}${path}`, {
        credentials: 'include',
        ...init,
        headers,
    })

    // Don't redirect to login if already on login page or making login/logout requests
    if (res.status === 401 && typeof window !== 'undefined') {
        const isLoginPage = window.location.pathname === '/login'
        const isAuthRequest = path.includes('/login') || path.includes('/logout')

        if (!isLoginPage && !isAuthRequest) {
            window.location.href = '/login'
        }
    }

    return res
}
