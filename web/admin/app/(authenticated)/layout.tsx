'use client'

import { ProtectedRoute } from '../lib/auth-context'
import { WebSocketProvider } from '../lib/websocket-context'
import TopNavigation from '../components/TopNavigation'

export default function AuthenticatedLayout({ children }: { children: React.ReactNode }) {
  return (
    <ProtectedRoute>
      <WebSocketProvider>
        <div className="flex h-screen flex-col overflow-hidden bg-slate-950">
          <TopNavigation />
          <main className="flex-1 overflow-auto">
            <div className="mx-auto max-w-full p-8">{children}</div>
          </main>
        </div>
      </WebSocketProvider>
    </ProtectedRoute>
  )
}