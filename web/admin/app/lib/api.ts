// Minimal fetch wrapper for our admin API:
// - routes through Next.js API proxy at /api/admin which forwards to backend
// - always sends cookies (credentials: 'include')
// - backend URL is configured via ADMIN_API_BASE env var at runtime

export async function apiFetch(path: string, init: RequestInit = {}) {
    const headers = new Headers(init.headers || {})
    if (!headers.has('Content-Type')) headers.set('Content-Type', 'application/json')

    // Route through Next.js proxy - /api/chains becomes /api/admin/chains
    const proxyPath = path.startsWith('/api/') ? path.replace('/api/', '/api/admin/') : `/api/admin${path}`

    const res = await fetch(proxyPath, {
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
