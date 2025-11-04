'use client'

import {createContext, useCallback, useContext, useMemo, useRef, useState} from 'react'
import clsx from 'clsx'

type ToastTone = 'info' | 'error'

type Toast = {
  id: number
  text: string
  tone: ToastTone
}

type ToastContextValue = {
  notify: (text: string, tone?: ToastTone) => void
  dismiss: (id: number) => void
  clear: () => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

export function ToastProvider({children}: {children: React.ReactNode}) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const timers = useRef<Map<number, ReturnType<typeof setTimeout>>>(new Map())

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id))
    const timer = timers.current.get(id)
    if (timer) {
      clearTimeout(timer)
      timers.current.delete(id)
    }
  }, [])

  const notify = useCallback(
    (text: string, tone: ToastTone = 'info') => {
      const id = Date.now() + Math.floor(Math.random() * 1000)
      setToasts((current) => {
        const next = current.slice(-3) // keep last few toasts
        return [...next, {id, text, tone}]
      })
      const timer = setTimeout(() => dismiss(id), 5000)
      timers.current.set(id, timer)
    },
    [dismiss],
  )

  const clear = useCallback(() => {
    timers.current.forEach((timer) => clearTimeout(timer))
    timers.current.clear()
    setToasts([])
  }, [])

  const value = useMemo(() => ({notify, dismiss, clear}), [notify, dismiss, clear])

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex max-w-sm flex-col-reverse gap-2">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={clsx(
              'pointer-events-auto flex items-start gap-3 rounded-lg border px-4 py-3 shadow-xl backdrop-blur-sm',
              toast.tone === 'error'
                ? 'border-rose-500/60 bg-rose-500/90 text-white'
                : 'border-sky-500/60 bg-sky-500/90 text-white',
            )}
          >
            <span className="flex-1 text-sm font-medium">{toast.text}</span>
            <button
              type="button"
              className="text-white/80 transition hover:text-white"
              onClick={() => dismiss(toast.id)}
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast() {
  const ctx = useContext(ToastContext)
  if (!ctx) {
    throw new Error('useToast must be used within a ToastProvider')
  }
  return ctx
}
