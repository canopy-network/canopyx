'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'

export default function RootPage() {
  const router = useRouter()

  useEffect(() => {
    // Redirect to dashboard - auth provider will handle redirect to login if not authenticated
    router.push('/dashboard')
  }, [router])

  return (
    <div className="flex h-screen items-center justify-center bg-slate-950">
      <div className="flex items-center gap-3">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-700 border-t-indigo-500"></div>
        <p className="text-slate-400">Loading...</p>
      </div>
    </div>
  )
}
