'use client'

import { useEffect, useRef, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'

type CreateChainFormData = {
  chain_id: number
  chain_name: string
  image: string
  rpc_endpoints: string[]
  min_replicas: number
  max_replicas: number
  reindex_min_replicas: number
  reindex_max_replicas: number
  reindex_scale_threshold: number
  notes?: string
}

interface CreateChainDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (data: CreateChainFormData) => Promise<void>
  saving: boolean
}

export default function CreateChainDialog({
  open,
  onOpenChange,
  onSubmit,
  saving,
}: CreateChainDialogProps) {
  const [chainId, setChainId] = useState('')
  const [chainName, setChainName] = useState('')
  const [image, setImage] = useState('')
  const [rpcEndpoints, setRpcEndpoints] = useState('')
  const [minReplicas, setMinReplicas] = useState(1)
  const [maxReplicas, setMaxReplicas] = useState(1)  // Changed default from 3 to 1
  const [reindexMinReplicas, setReindexMinReplicas] = useState(1)
  const [reindexMaxReplicas, setReindexMaxReplicas] = useState(3)
  const [reindexScaleThreshold, setReindexScaleThreshold] = useState(100)
  const [notes, setNotes] = useState('')
  const [error, setError] = useState('')
  const prevOpenRef = useRef(false)

  // Set defaults only when dialog transitions from closed to open
  useEffect(() => {
    if (open && !prevOpenRef.current) {
      // Dialog just opened - set defaults
      if (!image) setImage('canopynetwork/canopyx-indexer:latest')  // Changed to public image
      if (!rpcEndpoints) setRpcEndpoints('https://node1.canopy.us.nodefleet.net/rpc')
    }
    prevOpenRef.current = open
  }, [open])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    // RPC Endpoints validation (ONLY REQUIRED FIELD)
    const endpoints = rpcEndpoints
      .split('\n')
      .map((s) => s.trim())
      .filter((s) => s.length > 0)

    if (endpoints.length === 0) {
      setError('At least one RPC endpoint is required')
      return
    }

    // Min/Max Replicas validation
    if (minReplicas < 1) {
      setError('Minimum replicas must be at least 1')
      return
    }

    if (maxReplicas < minReplicas) {
      setError('Maximum replicas must be greater than or equal to minimum replicas')
      return
    }

    // Reindex Replicas validation
    if (reindexMinReplicas < 1) {
      setError('Reindex minimum replicas must be at least 1')
      return
    }

    if (reindexMaxReplicas < reindexMinReplicas) {
      setError('Reindex maximum replicas must be greater than or equal to reindex minimum replicas')
      return
    }

    if (reindexScaleThreshold < 1) {
      setError('Reindex scale threshold must be at least 1')
      return
    }

    // Chain ID validation (optional)
    const chainIdNum = chainId.trim() === '' ? 0 : Number(chainId)
    if (isNaN(chainIdNum) || chainIdNum < 0) {
      setError('Chain ID must be a positive number or empty for auto-detection')
      return
    }

    try {
      // Build payload - send empty strings for optional fields (backend will apply defaults)
      await onSubmit({
        chain_id: chainIdNum,
        chain_name: chainName.trim() || '',  // Empty string if not provided
        image: image.trim() || '',  // Empty string if not provided
        rpc_endpoints: endpoints,
        min_replicas: minReplicas,
        max_replicas: maxReplicas,
        reindex_min_replicas: reindexMinReplicas,
        reindex_max_replicas: reindexMaxReplicas,
        reindex_scale_threshold: reindexScaleThreshold,
        notes: notes.trim() || undefined,
      })
    } catch (err: any) {
      setError(err.message || 'Failed to create chain')
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/60 backdrop-blur-sm" />
        <Dialog.Content className="fixed left-1/2 top-1/2 w-full max-w-2xl -translate-x-1/2 -translate-y-1/2 rounded-xl border border-slate-800 bg-slate-900 p-6 shadow-2xl">
          <Dialog.Title className="text-xl font-bold text-white">Create New Chain</Dialog.Title>
          <Dialog.Description className="mt-2 text-sm text-slate-400">
            Add a new blockchain to the indexer infrastructure
          </Dialog.Description>

          <form onSubmit={handleSubmit} className="mt-6 space-y-4">
            {error && (
              <div className="rounded-lg border border-rose-500/50 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">
                {error}
              </div>
            )}

            <div className="grid gap-4 md:grid-cols-2">
              <div>
                <label htmlFor="chain_id" className="mb-2 block text-sm font-medium text-slate-300">
                  Chain ID
                </label>
                <input
                  id="chain_id"
                  type="number"
                  min="1"
                  value={chainId}
                  onChange={(e) => setChainId(e.target.value)}
                  className="input"
                  placeholder="Leave empty to auto-detect"
                />
                <p className="mt-1 text-xs text-slate-500">
                  Leave empty to auto-detect from RPC endpoints
                </p>
              </div>

              <div>
                <label htmlFor="chain_name" className="mb-2 block text-sm font-medium text-slate-300">
                  Chain Name
                  <span className="ml-2 text-xs font-normal text-slate-500">(Optional - defaults to &quot;Chain {'{id}'}&quot;)</span>
                </label>
                <input
                  id="chain_name"
                  type="text"
                  value={chainName}
                  onChange={(e) => setChainName(e.target.value)}
                  className="input"
                  placeholder="Leave empty to use default: Chain {id}"
                />
                <p className="mt-1 text-xs text-slate-500">Display name for this chain</p>
              </div>
            </div>

            <div>
              <label htmlFor="image" className="mb-2 block text-sm font-medium text-slate-300">
                Container Image
                <span className="ml-2 text-xs font-normal text-slate-500">(Optional - defaults to canopynetwork/canopyx-indexer:latest)</span>
              </label>
              <input
                id="image"
                type="text"
                value={image}
                onChange={(e) => setImage(e.target.value)}
                className="input"
                placeholder="canopynetwork/canopyx-indexer:latest"
              />
              <p className="mt-1 text-xs text-slate-500">
                Docker/container image for the indexer worker
              </p>
            </div>

            <div>
              <label htmlFor="rpc_endpoints" className="mb-2 block text-sm font-medium text-slate-300">
                RPC Endpoints <span className="text-rose-400">*</span>
              </label>
              <textarea
                id="rpc_endpoints"
                value={rpcEndpoints}
                onChange={(e) => setRpcEndpoints(e.target.value)}
                className="textarea"
                placeholder="https://eth-mainnet.g.alchemy.com/v2/YOUR-API-KEY&#10;https://mainnet.infura.io/v3/YOUR-API-KEY"
                rows={4}
                required
              />
              <p className="mt-1 text-xs text-slate-500">
                One endpoint per line. Multiple endpoints for redundancy.
              </p>
            </div>

            <div>
              <h3 className="text-sm font-semibold text-slate-300 mb-3">Regular Indexing</h3>
              <div className="grid gap-4 md:grid-cols-2">
                <div>
                  <label htmlFor="min_replicas" className="mb-2 block text-sm font-medium text-slate-300">
                    Min Replicas
                  </label>
                  <input
                    id="min_replicas"
                    type="number"
                    min={1}
                    value={minReplicas}
                    onChange={(e) => setMinReplicas(Number(e.target.value))}
                    className="input"
                  />
                  <p className="mt-1 text-xs text-slate-500">Minimum number of worker replicas</p>
                </div>

                <div>
                  <label htmlFor="max_replicas" className="mb-2 block text-sm font-medium text-slate-300">
                    Max Replicas
                  </label>
                  <input
                    id="max_replicas"
                    type="number"
                    min={minReplicas}
                    value={maxReplicas}
                    onChange={(e) => setMaxReplicas(Number(e.target.value))}
                    className="input"
                  />
                  <p className="mt-1 text-xs text-slate-500">Maximum number of worker replicas</p>
                </div>
              </div>
            </div>

            <div>
              <h3 className="text-sm font-semibold text-slate-300 mb-3">Reindex Settings</h3>
              <div className="grid gap-4 md:grid-cols-3">
                <div>
                  <label htmlFor="reindex_min_replicas" className="mb-2 block text-sm font-medium text-slate-300">
                    Min Replicas
                  </label>
                  <input
                    id="reindex_min_replicas"
                    type="number"
                    min={1}
                    value={reindexMinReplicas}
                    onChange={(e) => setReindexMinReplicas(Number(e.target.value))}
                    className="input"
                  />
                  <p className="mt-1 text-xs text-slate-500">Min replicas during reindex</p>
                </div>

                <div>
                  <label htmlFor="reindex_max_replicas" className="mb-2 block text-sm font-medium text-slate-300">
                    Max Replicas
                  </label>
                  <input
                    id="reindex_max_replicas"
                    type="number"
                    min={reindexMinReplicas}
                    value={reindexMaxReplicas}
                    onChange={(e) => setReindexMaxReplicas(Number(e.target.value))}
                    className="input"
                  />
                  <p className="mt-1 text-xs text-slate-500">Max replicas during reindex</p>
                </div>

                <div>
                  <label htmlFor="reindex_scale_threshold" className="mb-2 block text-sm font-medium text-slate-300">
                    Scale Threshold
                  </label>
                  <input
                    id="reindex_scale_threshold"
                    type="number"
                    min={1}
                    value={reindexScaleThreshold}
                    onChange={(e) => setReindexScaleThreshold(Number(e.target.value))}
                    className="input"
                  />
                  <p className="mt-1 text-xs text-slate-500">Queue depth to trigger scaling</p>
                </div>
              </div>
            </div>

            <div>
              <label htmlFor="notes" className="mb-2 block text-sm font-medium text-slate-300">
                Notes
              </label>
              <textarea
                id="notes"
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                className="textarea"
                placeholder="Optional notes about this chain..."
                rows={3}
              />
            </div>

            <div className="flex justify-end gap-3 pt-4">
              <Dialog.Close asChild>
                <button type="button" className="btn-secondary" disabled={saving}>
                  Cancel
                </button>
              </Dialog.Close>
              <button type="submit" className="btn" disabled={saving}>
                {saving ? (
                  <>
                    <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></div>
                    Creating...
                  </>
                ) : (
                  'Create Chain'
                )}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
