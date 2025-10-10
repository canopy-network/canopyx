'use client'

import {useCallback, useEffect, useMemo, useState} from 'react'
import * as Tabs from '@radix-ui/react-tabs'
import * as Dialog from '@radix-ui/react-dialog'
import * as Tooltip from '@radix-ui/react-tooltip'
import {InfoCircledIcon, ReloadIcon, Pencil1Icon, PauseIcon, PlayIcon} from '@radix-ui/react-icons'
import clsx from 'clsx'

import Nav from '../components/Nav'
import {apiFetch} from '../lib/api'

type ChainRow = {
  chain_id: string
  chain_name: string
  rpc_endpoints: string[]
  paused: number
  deleted?: number
  image: string
  min_replicas: number
  max_replicas: number
  notes?: string
}

type QueueStatus = {
  pending_workflow: number
  pending_activity: number
  backlog_age_secs: number
  pollers: number
}

type ChainStatus = {
  chain_id: string
  chain_name: string
  image: string
  notes?: string
  paused: boolean
  deleted: boolean
  min_replicas: number
  max_replicas: number
  last_indexed: number
  head: number
  queue: QueueStatus
}

type ChainStatusMap = Record<string, ChainStatus>

function formatNumber(n: number | undefined) {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}m`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return n.toLocaleString()
}

function secondsToFriendly(sec: number) {
  if (!sec || sec <= 0) return '—'
  if (sec < 60) return `${Math.round(sec)}s`
  if (sec < 3600) return `${Math.round(sec / 60)}m`
  return `${Math.round(sec / 3600)}h`
}

function QueueBadge({queue}: { queue: QueueStatus }) {
  const backlog = queue.pending_workflow + queue.pending_activity
  const tone = backlog > 500 ? 'bg-rose-500/20 text-rose-300' : backlog > 50 ? 'bg-amber-500/20 text-amber-300' : 'bg-emerald-500/20 text-emerald-300'
  return (
    <Tooltip.Root delayDuration={200}>
      <Tooltip.Trigger asChild>
        <span className={clsx('inline-flex items-center gap-1 px-2 py-1 rounded-full text-xs font-medium', tone)}>
          {formatNumber(backlog)} pending
        </span>
      </Tooltip.Trigger>
      <Tooltip.Content className="rounded bg-slate-800 px-3 py-2 text-sm shadow-lg" sideOffset={6}>
        <div className="space-y-1">
          <div className="flex items-center justify-between"><span className="text-slate-300">Workflow</span><span>{formatNumber(queue.pending_workflow)}</span></div>
          <div className="flex items-center justify-between"><span className="text-slate-300">Activity</span><span>{formatNumber(queue.pending_activity)}</span></div>
          <div className="flex items-center justify-between"><span className="text-slate-300">Pollers</span><span>{queue.pollers ?? 0}</span></div>
          <div className="flex items-center justify-between"><span className="text-slate-300">Oldest</span><span>{secondsToFriendly(queue.backlog_age_secs)}</span></div>
        </div>
        <Tooltip.Arrow className="fill-slate-800" />
      </Tooltip.Content>
    </Tooltip.Root>
  )
}

export default function ChainsPage() {
  const [chains, setChains] = useState<ChainRow[]>([])
  const [status, setStatus] = useState<ChainStatusMap>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>('')
  const [selected, setSelected] = useState<string | null>(null)
  const [dialogChain, setDialogChain] = useState<ChainRow | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [saving, setSaving] = useState(false)

  const loadChains = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await apiFetch('/api/chains')
      if (!res.ok) throw new Error('fetch')
      const list: ChainRow[] = await res.json()
      setChains(list)
    } catch (err) {
      console.error(err)
      setError('Unable to load chains. Please login again.')
    } finally {
      setLoading(false)
    }
  }, [])

  const loadStatus = useCallback(async (chainIds: string[]) => {
    if (!chainIds.length) return
    try {
      const res = await apiFetch(`/api/chains/status?ids=${encodeURIComponent(chainIds.join(','))}`)
      if (!res.ok) throw new Error('status')
      const data: ChainStatusMap = await res.json()
      setStatus(data)
      setChains((list) => {
        let mutated = false
        const next = list.map((c) => {
          const st = data[c.chain_id]
          if (!st) return c
          const paused = st.paused ? 1 : 0
          const deleted = st.deleted ? 1 : 0
          const minRep = st.min_replicas
          const maxRep = st.max_replicas
          const image = st.image || c.image
          const notes = st.notes ?? c.notes
          if (c.paused !== paused || (c.deleted ?? 0) !== deleted || c.min_replicas !== minRep || c.max_replicas !== maxRep || c.image !== image || c.notes !== notes) {
            mutated = true
            return {...c, paused, deleted, min_replicas: minRep, max_replicas: maxRep, image, notes}
          }
          return c
        })
        return mutated ? next : list
      })
    } catch (err) {
      console.warn('status polling failed', err)
    }
  }, [])

  useEffect(() => {
    loadChains()
  }, [loadChains])

  useEffect(() => {
    if (!chains.length) return
    loadStatus(chains.map((c) => c.chain_id))
    const interval = setInterval(() => loadStatus(chains.map((c) => c.chain_id)), 15_000)
    return () => clearInterval(interval)
  }, [chains, loadStatus])

  const rows = useMemo(() => {
    return chains.map((chain) => {
      const st = status[chain.chain_id]
      return {
        chain,
        status: st,
        lastIndexed: st?.last_indexed ?? 0,
        head: st?.head ?? 0,
        queue: st?.queue ?? {pending_activity: 0, pending_workflow: 0, backlog_age_secs: 0, pollers: 0},
      }
    })
  }, [chains, status])

  const togglePause = async (row: ChainRow) => {
    const nextPaused = row.paused ? 0 : 1
    try {
      await apiFetch('/api/chains/status', {
        method: 'PATCH',
        body: JSON.stringify([{chain_id: row.chain_id, paused: nextPaused}])
      })
      setChains((list) => list.map((c) => c.chain_id === row.chain_id ? {...c, paused: nextPaused} : c))
    } catch (err) {
      console.error(err)
      alert('Failed to update pause state')
    }
  }

  const openEditDialog = (row: ChainRow) => {
    setDialogChain(row)
    setDialogOpen(true)
  }

  const saveChain = async (updates: Partial<ChainRow>) => {
    if (!dialogChain) return
    setSaving(true)
    try {
      const payload = {
        ...dialogChain,
        ...updates,
      }
      await apiFetch('/api/chains', {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      setChains((list) => list.map((c) => c.chain_id === dialogChain.chain_id ? {...c, ...payload} : c))
      setDialogOpen(false)
    } catch (err) {
      console.error(err)
      alert('Failed to update chain')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="container">
        <Nav />
        <p className="mt-12 text-slate-300">Loading chains…</p>
      </div>
    )
  }

  if (error) {
    return (
      <div className="container">
        <Nav />
        <div className="mt-12 rounded bg-rose-500/10 border border-rose-500/40 px-4 py-3 text-rose-200">
          {error}
        </div>
      </div>
    )
  }

  return (
    <Tooltip.Provider delayDuration={200}>
    <div className="container">
      <Nav />
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-semibold">Chains</h1>
          <p className="text-sm text-slate-400">Operational view across indexer workers and queues.</p>
        </div>
        <button className="btn" onClick={() => loadChains()}>
          <ReloadIcon className="mr-2 h-4 w-4" /> Refresh
        </button>
      </div>

      <Tabs.Root defaultValue="overview" className="space-y-4">
        <Tabs.List className="flex gap-2 border-b border-slate-700">
          <Tabs.Trigger value="overview" className="px-3 py-2 text-sm data-[state=active]:border-b-2 data-[state=active]:border-sky-400 data-[state=active]:text-sky-300">
            Overview
          </Tabs.Trigger>
          <Tabs.Trigger value="selected" className="px-3 py-2 text-sm data-[state=active]:border-b-2 data-[state=active]:border-sky-400 data-[state=active]:text-sky-300" disabled={!selected}>
            Selection
          </Tabs.Trigger>
        </Tabs.List>

        <Tabs.Content value="overview">
          <div className="overflow-x-auto rounded border border-slate-800">
            <table className="min-w-full text-sm">
              <thead className="bg-slate-900/60 text-slate-300">
                <tr>
                  <th className="px-3 py-2 text-left">Chain</th>
                  <th className="px-3 py-2 text-left">Queue</th>
                  <th className="px-3 py-2 text-left">Indexed</th>
                  <th className="px-3 py-2 text-left">Head</th>
                  <th className="px-3 py-2 text-left">Replicas</th>
                  <th className="px-3 py-2 text-left">Actions</th>
                </tr>
              </thead>
              <tbody>
                {rows.map(({chain, status, lastIndexed, head, queue}) => {
                  const isSelected = selected === chain.chain_id
                  return (
                    <tr key={chain.chain_id} className={clsx('border-t border-slate-800', isSelected && 'bg-slate-800/40')}>
                      <td className="px-3 py-3">
                        <button className="text-left" onClick={() => setSelected(isSelected ? null : chain.chain_id)}>
                          <div className="font-medium text-slate-100">{chain.chain_name || chain.chain_id}</div>
                          <div className="text-xs text-slate-500">{chain.chain_id}</div>
                          {chain.notes && <div className="mt-1 text-xs text-slate-400">{chain.notes}</div>}
                        </button>
                      </td>
                      <td className="px-3 py-3"><QueueBadge queue={queue} /></td>
                      <td className="px-3 py-3">{formatNumber(lastIndexed)}</td>
                      <td className="px-3 py-3">{formatNumber(head)}</td>
                      <td className="px-3 py-3 text-sm text-slate-300">
                        <div className="flex items-center gap-2">
                          <span>{chain.min_replicas}</span>
                          <span className="text-slate-500">→</span>
                          <span>{chain.max_replicas}</span>
                        </div>
                      </td>
                      <td className="px-3 py-3">
                        <div className="flex items-center gap-2">
                          <button className="btn-secondary" onClick={() => togglePause(chain)}>
                            {chain.paused ? <PlayIcon /> : <PauseIcon />}
                            <span>{chain.paused ? 'Resume' : 'Pause'}</span>
                          </button>
                          <button className="btn-secondary" onClick={() => openEditDialog(chain)}>
                            <Pencil1Icon /> Edit
                          </button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </Tabs.Content>

        <Tabs.Content value="selected">
          {selected ? (
            <SelectionPane
              chain={chains.find((c) => c.chain_id === selected)!}
              status={status[selected]}
              onEdit={() => openEditDialog(chains.find((c) => c.chain_id === selected)!)}
            />
          ) : (
            <div className="rounded border border-slate-800 px-4 py-8 text-center text-slate-400">
              Select a chain from the table to inspect details.
            </div>
          )}
        </Tabs.Content>
      </Tabs.Root>

      <EditChainDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        chain={dialogChain}
        onSave={saveChain}
        saving={saving}
      />
    </div>
    </Tooltip.Provider>
  )
}

interface SelectionPaneProps {
  chain: ChainRow
  status?: ChainStatus
  onEdit: () => void
}

function SelectionPane({chain, status, onEdit}: SelectionPaneProps) {
  const queue = status?.queue
  const paused = status ? status.paused : chain.paused === 1
  const minReplicas = status?.min_replicas ?? chain.min_replicas
  const maxReplicas = status?.max_replicas ?? chain.max_replicas
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <div className="rounded border border-slate-800 p-4">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">Overview</h3>
          <button className="btn-secondary" onClick={onEdit}><Pencil1Icon /> Edit</button>
        </div>
        <dl className="mt-4 space-y-2 text-sm">
          <div className="flex items-center justify-between"><dt className="text-slate-400">Chain ID</dt><dd>{chain.chain_id}</dd></div>
          <div className="flex items-center justify-between"><dt className="text-slate-400">Image</dt><dd>{status?.image || chain.image || '—'}</dd></div>
          <div className="flex items-center justify-between"><dt className="text-slate-400">Replicas</dt><dd>{minReplicas} &rarr; {maxReplicas}</dd></div>
          <div className="flex items-center justify-between"><dt className="text-slate-400">Paused</dt><dd>{paused ? 'Yes' : 'No'}</dd></div>
        </dl>
        {chain.rpc_endpoints?.length ? (
          <div className="mt-4">
            <div className="text-sm text-slate-400">RPC Endpoints</div>
            <ul className="mt-2 space-y-1 text-sm text-slate-300">
              {chain.rpc_endpoints.map((ep) => <li key={ep}>{ep}</li>)}
            </ul>
          </div>
        ) : null}
      </div>
      <div className="rounded border border-slate-800 p-4 space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">Queue</h3>
          <Tooltip.Root delayDuration={200}>
            <Tooltip.Trigger asChild>
              <InfoCircledIcon className="text-slate-500" />
            </Tooltip.Trigger>
            <Tooltip.Content className="rounded bg-slate-800 px-3 py-2 text-sm shadow-lg" sideOffset={6}>
              Metrics pulled from Temporal task queue `index:{chain.chain_id}`.
              <Tooltip.Arrow className="fill-slate-800" />
            </Tooltip.Content>
          </Tooltip.Root>
        </div>
        <dl className="grid grid-cols-2 gap-3 text-sm">
          <div><dt className="text-slate-400">Workflow backlog</dt><dd className="text-slate-100">{formatNumber(queue?.pending_workflow ?? 0)}</dd></div>
          <div><dt className="text-slate-400">Activity backlog</dt><dd className="text-slate-100">{formatNumber(queue?.pending_activity ?? 0)}</dd></div>
          <div><dt className="text-slate-400">Age</dt><dd className="text-slate-100">{secondsToFriendly(queue?.backlog_age_secs ?? 0)}</dd></div>
          <div><dt className="text-slate-400">Pollers</dt><dd className="text-slate-100">{queue?.pollers ?? 0}</dd></div>
        </dl>
        <div className="rounded bg-slate-900/60 px-4 py-3 text-sm text-slate-300">
          <div className="flex items-center justify-between"><span>Indexed height</span><span>{formatNumber(status?.last_indexed ?? 0)}</span></div>
          <div className="mt-1 flex items-center justify-between"><span>Head height</span><span>{formatNumber(status?.head ?? 0)}</span></div>
        </div>
      </div>
    </div>
  )
}

interface EditDialogProps {
  chain: ChainRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (updates: Partial<ChainRow>) => Promise<void>
  saving: boolean
}

function EditChainDialog({chain, open, onOpenChange, onSave, saving}: EditDialogProps) {
  const [image, setImage] = useState('')
  const [minReplicas, setMinReplicas] = useState(1)
  const [maxReplicas, setMaxReplicas] = useState(1)
  const [notes, setNotes] = useState('')

  useEffect(() => {
    if (!chain) return
    setImage(chain.image || '')
    setMinReplicas(chain.min_replicas)
    setMaxReplicas(chain.max_replicas)
    setNotes(chain.notes || '')
  }, [chain])

  const submit = async () => {
    if (!chain) return
    await onSave({
      ...chain,
      image,
      min_replicas: Number(minReplicas),
      max_replicas: Number(maxReplicas),
      notes,
    })
  }

  return (
    <Dialog.Root open={open && !!chain} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60" />
        <Dialog.Content className="fixed left-1/2 top-1/2 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-lg border border-slate-800 bg-slate-900 p-6 shadow-xl">
          <Dialog.Title className="text-lg font-semibold">Edit chain</Dialog.Title>
          <Dialog.Description className="text-sm text-slate-400 mt-1">Update deployment parameters for {chain?.chain_name || chain?.chain_id}.</Dialog.Description>
          <div className="mt-4 space-y-3 text-sm">
            <label className="block">
              <span className="text-slate-300">Container image</span>
              <input className="input mt-1" value={image} onChange={(e) => setImage(e.target.value)} placeholder="ghcr.io/..." />
            </label>
            <div className="grid grid-cols-2 gap-3">
              <label className="block">
                <span className="text-slate-300">Min replicas</span>
                <input className="input mt-1" type="number" min={1} value={minReplicas} onChange={(e) => setMinReplicas(Number(e.target.value))} />
              </label>
              <label className="block">
                <span className="text-slate-300">Max replicas</span>
                <input className="input mt-1" type="number" min={minReplicas} value={maxReplicas} onChange={(e) => setMaxReplicas(Number(e.target.value))} />
              </label>
            </div>
            <label className="block">
              <span className="text-slate-300">Notes</span>
              <textarea className="input mt-1 h-24" value={notes} onChange={(e) => setNotes(e.target.value)} />
            </label>
          </div>
          <div className="mt-6 flex justify-end gap-2">
            <Dialog.Close asChild>
              <button className="btn-secondary" disabled={saving}>Cancel</button>
            </Dialog.Close>
            <button className="btn" onClick={submit} disabled={saving || !image.trim()}>
              {saving ? 'Saving…' : 'Save changes'}
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
