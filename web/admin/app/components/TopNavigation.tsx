'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useAuth } from '../lib/auth-context'

const navigation = [
  {
    name: 'Dashboard',
    href: '/dashboard',
    icon: (
      <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"
        />
      </svg>
    ),
  },
]

export default function TopNavigation() {
  const pathname = usePathname()
  const { logout } = useAuth()

  return (
    <header className="flex h-16 items-center justify-between border-b border-slate-800 bg-slate-900 px-6">
      {/* Left side - Logo and Navigation */}
      <div className="flex items-center gap-8">
        {/* Logo */}
        <Link href="/dashboard" className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br from-indigo-500 to-purple-600 shadow-lg shadow-indigo-500/20">
            <svg
              className="h-6 w-6 text-white"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"
              />
            </svg>
          </div>
          <div>
            <h1 className="text-lg font-bold text-white">CanopyX</h1>
            <p className="text-xs text-slate-400">Admin Console</p>
          </div>
        </Link>

        {/* Navigation Links */}
        <nav className="flex items-center gap-2">
          {navigation.map((item) => {
            const isActive = pathname?.startsWith(item.href)
            return (
              <Link
                key={item.name}
                href={item.href}
                className={`flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all ${
                  isActive
                    ? 'bg-gradient-to-r from-indigo-500/10 to-purple-500/10 text-white shadow-sm'
                    : 'text-slate-400 hover:bg-slate-800/50 hover:text-white'
                }`}
              >
                <span
                  className={isActive ? 'text-indigo-400' : 'text-slate-500'}
                >
                  {item.icon}
                </span>
                {item.name}
                {isActive && (
                  <span className="ml-1 h-1.5 w-1.5 rounded-full bg-indigo-400"></span>
                )}
              </Link>
            )
          })}
        </nav>
      </div>

      {/* Right side - User section */}
      <div className="flex items-center gap-3">
        <div className="flex items-center gap-3 rounded-lg bg-slate-800/50 px-3 py-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-full bg-gradient-to-br from-indigo-500 to-purple-600 text-xs font-semibold text-white">
            A
          </div>
          <div>
            <p className="text-sm font-medium text-white">Admin</p>
            <p className="text-xs text-slate-400">Administrator</p>
          </div>
        </div>
        <button
          onClick={() => logout()}
          className="flex items-center gap-2 rounded-lg border border-slate-700 bg-slate-800/50 px-3 py-2 text-sm font-medium text-slate-300 transition-all hover:border-slate-600 hover:bg-slate-800 hover:text-white"
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
            />
          </svg>
          Sign Out
        </button>
      </div>
    </header>
  )
}