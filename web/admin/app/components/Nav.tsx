'use client'
import Link from 'next/link'
import {usePathname} from 'next/navigation'
import {useState} from 'react'
import {apiFetch} from '../lib/api'

export default function Nav() {
    const p = usePathname()
    const [loggingOut, setLoggingOut] = useState(false)

    const link = (href: string, txt: string) => (
        <Link
            href={href}
            className={`px-3 py-1 rounded ${p?.startsWith(href) ? 'text-sky-400' : 'opacity-80 hover:opacity-100'}`}
        >
            {txt}
        </Link>
    )

    const onLogout = async () => {
        if (loggingOut) return
        setLoggingOut(true)
        try {
            await apiFetch('/logout', {method: 'POST'})
        } catch {
            /* ignore */
        }
        // Hard redirect to clear all client state.
        window.location.href = '/login'
    }
    const Item = ({href, children}: { href: string, children: any }) => (
        <Link
            className={"px-3 py-2 rounded-md " + (p?.startsWith(href) ? "bg-slate-700 text-white" : "text-sky-400 hover:text-sky-300")}
            href={href}>{children}</Link>
    )
    return (
        <nav className="flex items-center justify-between py-4">
            <div className="flex items-center gap-2">
                {link('/', 'Dashboard')}
                {link('/chains', 'Chains')}
            </div>
            <div/>
            <div className="flex items-center gap-2">
                <button className="btn" onClick={onLogout} disabled={loggingOut} title="Log out">
                    {loggingOut ? 'Logging out…' : 'Logout'}
                </button>
            </div>
        </nav>
    )
}
