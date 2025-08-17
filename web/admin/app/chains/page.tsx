// noinspection ExceptionCaughtLocallyJS

'use client'

import {useEffect, useMemo, useState} from 'react'
import Link from 'next/link'
import {apiFetch} from '../lib/api'
import Nav from "../components/Nav";

type Chain = {
    chain_id: string
    chain_name: string
    rpc_endpoints: string[]
    paused: number
    deleted?: number
}

type BulkStatusRow = {
    last_indexed?: number
    head?: number
}

type ProgressResp = { last: number } // from /chains/{id}/progress

export default function Chains() {
    const [chains, setChains] = useState<Chain[]>([])
    const [err, setErr] = useState('')
    const [loading, setLoading] = useState(true)
    const [updating, setUpdating] = useState<string | null>(null)
    const [bulkUpdating, setBulkUpdating] = useState(false)
    const [sel, setSel] = useState<Record<string, boolean>>({})
    const [indexed, setIndexed] = useState<Record<string, number>>({})
    const [head, setHead] = useState<Record<string, number>>({})

    const selectedIds = useMemo(
        () => Object.entries(sel).filter(([, v]) => v).map(([k]) => k),
        [sel]
    )

    // helpers
    const setMany = <T extends string | number>(
        setter: React.Dispatch<React.SetStateAction<Record<string, T>>>,
        rows: Array<{ id: string; value: T }>
    ) => {
        setter((m) => {
            const next = {...m}
            for (const r of rows) next[r.id] = r.value
            return next
        })
    }

    // 1) load chains
    useEffect(() => {
        let cancelled = false
        setLoading(true)
        apiFetch('/api/chains')
            .then((r) => (r.ok ? r.json() : Promise.reject(new Error('auth'))))
            .then((list: Chain[]) => {
                if (cancelled) return
                setChains(list)
                // seed selection map (all unchecked)
                const base: Record<string, boolean> = {}
                list.forEach((c) => (base[c.chain_id] = false))
                setSel(base)
            })
            .catch(() => setErr('Not authorized. Please login again.'))
            .finally(() => !cancelled && setLoading(false))
        return () => {
            cancelled = true
        }
    }, [])

    // 2) poll heights (bulk → fallback) every 10s, pause when tab hidden
    useEffect(() => {
        if (!chains.length) return
        const POLL_MS = 10_000
        let controller: AbortController | null = null

        const loadOnce = async () => {
            controller?.abort()
            controller = new AbortController()

            const ids = chains.map((c) => c.chain_id)
            const qs = encodeURIComponent(ids.join(','))
            try {
                const r = await apiFetch(`/api/chains/status?ids=${qs}`, {
                    headers: {'Content-Type': 'application/json'},
                    signal: controller.signal,
                })
                if (!r.ok) throw new Error('no-bulk')
                const data: Record<string, BulkStatusRow> = await r.json()
                const idxRows = Object.entries(data)
                    .filter(([, v]) => typeof v.last_indexed === 'number')
                    .map(([id, v]) => ({id, value: v.last_indexed as number}))
                const headRows = Object.entries(data)
                    .filter(([, v]) => typeof v.head === 'number')
                    .map(([id, v]) => ({id, value: v.head as number}))
                if (idxRows.length) setMany(setIndexed, idxRows)
                if (headRows.length) setMany(setHead, headRows)
            } catch {
                // fallback: per-chain progress endpoint for "indexed"
                const rows = await Promise.all(
                    chains.map((c) =>
                        apiFetch(`/api/chains/${encodeURIComponent(c.chain_id)}/progress`, {
                            headers: {'Content-Type': 'application/json'},
                            signal: controller!.signal,
                        })
                            .then((r) => (r.ok ? r.json() : {last: 0}))
                            .then((p: ProgressResp) => ({id: c.chain_id, value: p.last || 0}))
                            .catch(() => ({id: c.chain_id, value: 0}))
                    )
                )
                setMany(setIndexed, rows)
            }
        }

        loadOnce()

        const tick = () => {
            if (document.hidden) return
            loadOnce()
        }
        const iv = setInterval(tick, POLL_MS)
        const onVis = () => {
            if (!document.hidden) loadOnce()
        }
        document.addEventListener('visibilitychange', onVis)

        return () => {
            clearInterval(iv)
            document.removeEventListener('visibilitychange', onVis)
            controller?.abort()
        }
    }, [chains])

    // actions
    const togglePause = async (c: Chain) => {
        setUpdating(c.chain_id)
        try {
            const body = [{chain_id: c.chain_id, paused: (!c.paused) ? 1 : 0}]
            const r = await apiFetch('/api/chains/status', {
                method: 'PATCH',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify(body),
            })
            if (!r.ok) throw new Error(await r.text())
            setChains((list) =>
                list.map((x) => (x.chain_id === c.chain_id ? {...x, paused: x.paused == 0 ? 1 : 0} : x))
            )
        } catch (e) {
            console.error(e)
            alert('Failed to update pause state')
        } finally {
            setUpdating(null)
        }
    }

    const deleteChain = async (c: Chain) => {
        if (!confirm(`Delete chain ${c.chain_id}?`)) return
        setUpdating(c.chain_id)
        try {
            const body = [{chain_id: c.chain_id, deleted: 1}]
            const r = await apiFetch('/api/chains/status', {
                method: 'PATCH',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify(body),
            })
            if (!r.ok) throw new Error(await r.text())
            setChains((list) => list.filter((x) => x.chain_id !== c.chain_id))
        } catch (e) {
            console.error(e)
            alert('Failed to delete')
        } finally {
            setUpdating(null)
        }
    }

    const bulkPause = async (paused: number) => {
        if (!selectedIds.length) return
        setBulkUpdating(true)
        try {
            const body = selectedIds.map((id) => ({chain_id: id, paused: paused ? 1 : 0}))
            const r = await apiFetch('/api/chains/status', {
                method: 'PATCH',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify(body),
            })
            if (!r.ok) throw new Error(await r.text())
            setChains((list) =>
                list.map((c) => (sel[c.chain_id] ? {...c, paused} : c))
            )
            setSel((m) => {
                const n = {...m}
                selectedIds.forEach((id) => (n[id] = false))
                return n
            })
        } catch (e) {
            console.error(e)
            alert('Bulk update failed')
        } finally {
            setBulkUpdating(false)
        }
    }

    const allChecked = useMemo(
        () => chains.length > 0 && chains.every((c) => sel[c.chain_id]),
        [chains, sel]
    )
    const toggleAll = (checked: boolean) => {
        setSel((m) => {
            const n = {...m}
            chains.forEach((c) => (n[c.chain_id] = checked))
            return n
        })
    }

    return (
        <div className="container mx-auto px-4 py-6">
            <Nav/>
            <div className="card">

                <div className="flex items-center justify-between mb-4">
                    <h1 className="text-xl font-semibold">Chains</h1>
                    <div className="flex items-center gap-2">
                        <Link className="btn" href="/new">+ New</Link>
                        <button
                            className="btn"
                            disabled={!selectedIds.length || bulkUpdating}
                            onClick={() => bulkPause(1)}
                        >
                            Pause selected
                        </button>
                        <button
                            className="btn"
                            disabled={!selectedIds.length || bulkUpdating}
                            onClick={() => bulkPause(0)}
                        >
                            Resume selected
                        </button>
                    </div>
                </div>

                {err && <p className="text-rose-400 mb-3">{err}</p>}
                {loading ? (
                    <p className="opacity-70">Loading…</p>
                ) : (
                    <div className="card overflow-x-auto">
                        <table className="table w-full">
                            <thead>
                            <tr>
                                <th className="w-10">
                                    <input
                                        type="checkbox"
                                        checked={allChecked}
                                        onChange={(e) => toggleAll(e.target.checked)}
                                    />
                                </th>
                                <th>ID</th>
                                <th>Name</th>
                                <th>RPCs</th>
                                <th className="text-right">Indexed</th>
                                <th className="text-right">Head</th>
                                <th className="text-right">Lag</th>
                                <th>Status</th>
                                <th className="w-56"></th>
                            </tr>
                            </thead>
                            <tbody>
                            {chains.map((c) => {
                                const idx = indexed[c.chain_id] ?? 0
                                const hd = head[c.chain_id] ?? 0
                                const lag = hd && idx ? Math.max(hd - idx, 0) : 0
                                return (
                                    <tr key={c.chain_id} className={c.paused ? 'opacity-70' : ''}>
                                        <td>
                                            <input
                                                type="checkbox"
                                                checked={!!sel[c.chain_id]}
                                                onChange={(e) =>
                                                    setSel((m) => ({...m, [c.chain_id]: e.target.checked}))
                                                }
                                            />
                                        </td>
                                        <td className="font-mono">{c.chain_id}</td>
                                        <td>{c.chain_name}</td>
                                        <td className="text-slate-300 truncate max-w-[420px]">
                                            {c.rpc_endpoints?.join(', ')}
                                        </td>
                                        <td className="text-right tabular-nums">{idx || '-'}</td>
                                        <td className="text-right tabular-nums">{hd || '-'}</td>
                                        <td className="text-right tabular-nums">
                                            {lag ? (
                                                <span className={lag > 1000 ? 'text-rose-400' : 'text-emerald-400'}>
                          {lag}
                        </span>
                                            ) : '-'}
                                        </td>
                                        <td>
                                            {c.paused ? (
                                                <span
                                                    className="inline-block px-2 py-0.5 rounded bg-yellow-500/20 text-yellow-300 text-xs">
                          paused
                        </span>
                                            ) : (
                                                <span
                                                    className="inline-block px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300 text-xs">
                          active
                        </span>
                                            )}
                                        </td>
                                        <td className="text-right">
                                            <div className="flex items-center gap-2 justify-end">
                                                <Link className="link" href={`/chains/view?id=${c.chain_id}`}>
                                                    View
                                                </Link>
                                                <button
                                                    className="btn"
                                                    disabled={updating === c.chain_id}
                                                    onClick={() => togglePause(c)}
                                                >
                                                    {c.paused ? 'Resume' : 'Pause'}
                                                </button>
                                                <button
                                                    className="btn"
                                                    disabled={updating === c.chain_id}
                                                    onClick={() => deleteChain(c)}
                                                >
                                                    Delete
                                                </button>
                                            </div>
                                        </td>
                                    </tr>
                                )
                            })}
                            {!chains.length && (
                                <tr>
                                    <td colSpan={9} className="py-8 text-center opacity-70">
                                        No chains yet. Click “New” to add one.
                                    </td>
                                </tr>
                            )}
                            </tbody>
                        </table>
                    </div>
                )}
            </div>
        </div>
    )
}
