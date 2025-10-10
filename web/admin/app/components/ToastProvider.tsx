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
      <div className="pointer-events-none fixed top-4 right-4 z-50 flex max-w-sm flex-col gap-2">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={clsx(
              'pointer-events-auto flex items-start gap-3 rounded border px-4 py-3 shadow-lg',
              toast.tone === 'error'
                ? 'border-rose-500/40 bg-rose-500/10 text-rose-100'
                : 'border-sky-500/40 bg-sky-500/10 text-sky-100',
            )}
          >
            <span className="flex-1 text-sm">{toast.text}</span>
            <button
              type="button"
              className="text-xs text-slate-400 transition hover:text-slate-200"
              onClick={() => dismiss(toast.id)}
            >
              Close
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
