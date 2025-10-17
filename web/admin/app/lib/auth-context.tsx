'use client'

import { createContext, useContext, useEffect, useState } from 'react'
import { useRouter, usePathname } from 'next/navigation'
import { apiFetch } from './api'

interface AuthContextType {
  isAuthenticated: boolean
  isLoading: boolean
  login: (username: string, password: string) => Promise<{ success: boolean; error?: string }>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const router = useRouter()
  const pathname = usePathname()

  useEffect(() => {
    const checkAuth = async () => {
      try {
        // Try to fetch chains to check if authenticated
        const res = await apiFetch('/api/chains', { method: 'GET' })
        if (res.ok) {
          setIsAuthenticated(true)
          // If on login page and authenticated, redirect to dashboard
          if (pathname === '/login') {
            router.push('/dashboard')
          }
        } else {
          setIsAuthenticated(false)
          // If not on login page and not authenticated, redirect to login
          if (pathname !== '/login') {
            router.push('/login')
          }
        }
      } catch (error) {
        setIsAuthenticated(false)
        if (pathname !== '/login') {
          router.push('/login')
        }
      } finally {
        setIsLoading(false)
      }
    }

    checkAuth()
  }, [pathname, router])

  const login = async (username: string, password: string) => {
    try {
      const res = await apiFetch('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      })

      if (res.ok) {
        setIsAuthenticated(true)
        return { success: true }
      } else {
        const data = await res.json().catch(() => ({ error: 'Login failed' }))
        return { success: false, error: data.error || 'Login failed' }
      }
    } catch (error) {
      return { success: false, error: 'Network error' }
    }
  }

  const logout = async () => {
    try {
      // Set authenticated to false first to prevent race condition
      setIsAuthenticated(false)
      await apiFetch('/api/auth/logout', { method: 'POST' })
    } catch (error) {
      // Ignore errors during logout
    } finally {
      router.push('/login')
    }
  }

  return (
    <AuthContext.Provider value={{ isAuthenticated, isLoading, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}

export function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth()

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-950">
        <div className="flex items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
          <p className="text-slate-400">Loading...</p>
        </div>
      </div>
    )
  }

  if (!isAuthenticated) {
    return null
  }

  return <>{children}</>
}
