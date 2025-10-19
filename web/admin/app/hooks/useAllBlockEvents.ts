import { useEffect, useState } from 'react'
import { useWebSocketContext } from '../lib/websocket-context'
import { BlockIndexedEvent } from './useWebSocket'

interface UseAllBlockEventsOptions {
  enabled?: boolean
}

interface UseAllBlockEventsReturn {
  // Map of chainId -> latest block event
  chainEvents: Record<string, BlockIndexedEvent>

  // Connection status
  isConnected: boolean
  error: string | null
  reconnectAttempts: number
}

/**
 * Hook for consuming block events for ALL chains
 *
 * Automatically subscribes to block events for all chains using wildcard pattern
 * and provides the latest event data for each chain. Manages subscription lifecycle
 * automatically based on component mount/unmount.
 *
 * Perfect for dashboard views that need to show real-time updates for all chains.
 *
 * @example
 * ```tsx
 * const { chainEvents, isConnected } = useAllBlockEvents({
 *   enabled: true
 * })
 *
 * // Update chain status when new blocks arrive
 * useEffect(() => {
 *   Object.entries(chainEvents).forEach(([chainId, event]) => {
 *     updateChainStatus(chainId, event.height)
 *   })
 * }, [chainEvents])
 * ```
 */
export function useAllBlockEvents({
  enabled = true,
}: UseAllBlockEventsOptions = {}): UseAllBlockEventsReturn {
  const {
    isConnected,
    error,
    reconnectAttempts,
    chainEventsMap,
    subscribe,
    unsubscribe,
  } = useWebSocketContext()

  // Local state to track events for all chains
  const [chainEvents, setChainEvents] = useState<Record<string, BlockIndexedEvent>>({})

  // Subscribe to wildcard on mount, unsubscribe on unmount
  useEffect(() => {
    if (!enabled) {
      return
    }

    console.log('[useAllBlockEvents] Subscribing to all chains with pattern: *')
    subscribe('*')

    return () => {
      console.log('[useAllBlockEvents] Unsubscribing from all chains')
      unsubscribe('*')
    }
  }, [enabled, subscribe, unsubscribe])

  // Convert Map to Record whenever the Map changes
  // This provides a stable object reference that React can detect changes on
  useEffect(() => {
    if (!enabled) {
      return
    }

    // Convert Map to Record
    const updates: Record<string, BlockIndexedEvent> = {}
    let hasUpdates = false

    chainEventsMap.forEach((event, chainId) => {
      updates[chainId] = event

      // Check if this is a new or updated event
      if (!chainEvents[chainId] || chainEvents[chainId].height !== event.height) {
        hasUpdates = true
      }
    })

    // Only update state if there are actual changes
    if (hasUpdates) {
      setChainEvents(updates)
    }
  }, [chainEventsMap, chainEvents, enabled])

  return {
    chainEvents,
    isConnected,
    error,
    reconnectAttempts,
  }
}