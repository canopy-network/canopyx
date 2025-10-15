'use client'

import { useEffect, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'

type CreateChainFormData = {
  chain_id: string
  chain_name: string
  image: string
  rpc_endpoints: string[]
  min_replicas: number
  max_replicas: number
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
  const [image, setImage] = useState('localhost:5001/canopyx-indexer:dev')
  const [rpcEndpoints, setRpcEndpoints] = useState('https://node1.canopy.us.nodefleet.net/rpc')
  const [minReplicas, setMinReplicas] = useState(1)
  const [maxReplicas, setMaxReplicas] = useState(3)
  const [notes, setNotes] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) {
      // Reset form when dialog closes
      setChainId('')
      setChainName('')
      setImage('')
      setRpcEndpoints('')
      setMinReplicas(1)
      setMaxReplicas(3)
      setNotes('')
      setError('')
    }
  }, [open])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    // Validate
    if (!chainId.trim()) {
      setError('Chain ID is required')
      return
    }
    if (!chainName.trim()) {
      setError('Chain name is required')
      return
    }
    if (!image.trim()) {
      setError('Container image is required')
      return
    }

    const endpoints = rpcEndpoints
      .split('\n')
      .map((s) => s.trim())
      .filter((s) => s.length > 0)

    if (endpoints.length === 0) {
      setError('At least one RPC endpoint is required')
      return
    }

    if (minReplicas < 1) {
      setError('Minimum replicas must be at least 1')
      return
    }

    if (maxReplicas < minReplicas) {
      setError('Maximum replicas must be greater than or equal to minimum replicas')
      return
    }

    try {
      await onSubmit({
        chain_id: chainId,
        chain_name: chainName,
        image,
        rpc_endpoints: endpoints,
        min_replicas: minReplicas,
        max_replicas: maxReplicas,
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
                  Chain ID <span className="text-rose-400">*</span>
                </label>
                <input
                  id="chain_id"
                  type="text"
                  value={chainId}
                  onChange={(e) => setChainId(e.target.value)}
                  className="input"
                  placeholder="e.g., ethereum_mainnet"
                  required
                />
                <p className="mt-1 text-xs text-slate-500">Unique identifier for this chain</p>
              </div>

              <div>
                <label htmlFor="chain_name" className="mb-2 block text-sm font-medium text-slate-300">
                  Chain Name <span className="text-rose-400">*</span>
                </label>
                <input
                  id="chain_name"
                  type="text"
                  value={chainName}
                  onChange={(e) => setChainName(e.target.value)}
                  className="input"
                  placeholder="e.g., Ethereum Mainnet"
                  required
                />
                <p className="mt-1 text-xs text-slate-500">Display name for this chain</p>
              </div>
            </div>

            <div>
              <label htmlFor="image" className="mb-2 block text-sm font-medium text-slate-300">
                Container Image <span className="text-rose-400">*</span>
              </label>
              <input
                id="image"
                type="text"
                value={image}
                onChange={(e) => setImage(e.target.value)}
                className="input"
                placeholder="e.g., ghcr.io/myorg/indexer:latest"
                required
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
