import { useEffect, useState } from 'react'
import { useWebSocketContext } from '../lib/websocket-context'
import { BlockIndexedEvent } from './useWebSocket'

interface UseBlockEventsOptions {
  chainId: string
  enabled?: boolean
}

interface UseBlockEventsReturn {
  // Latest block event for this chain
  lastEvent: BlockIndexedEvent | null

  // Connection status
  isConnected: boolean
  error: string | null
  reconnectAttempts: number

  // Convenience properties from last event
  lastHeight: number | null
  lastTimestamp: string | null
}

/**
 * Hook for consuming block events for a specific chain
 *
 * Automatically subscribes to block events for the given chainId
 * and provides the latest event data. Manages subscription lifecycle
 * automatically based on component mount/unmount.
 *
 * @example
 * ```tsx
 * const { lastEvent, lastHeight, isConnected } = useBlockEvents({
 *   chainId: 'canopy_local'
 * })
 *
 * useEffect(() => {
 *   if (lastHeight) {
 *     console.log('New block indexed:', lastHeight)
 *     refetchData()
 *   }
 * }, [lastHeight])
 * ```
 */
export function useBlockEvents({
  chainId,
  enabled = true,
}: UseBlockEventsOptions): UseBlockEventsReturn {
  const {
    isConnected,
    error,
    reconnectAttempts,
    getLastEventForChain,
    subscribe,
    unsubscribe,
  } = useWebSocketContext()

  // Local state to track the last event for this chain
  const [lastEvent, setLastEvent] = useState<BlockIndexedEvent | null>(null)

  // Subscribe on mount, unsubscribe on unmount
  useEffect(() => {
    if (!enabled) {
      return
    }

    console.log(`[useBlockEvents] Subscribing to chain: ${chainId}`)
    subscribe(chainId)

    return () => {
      console.log(`[useBlockEvents] Unsubscribing from chain: ${chainId}`)
      unsubscribe(chainId)
    }
  }, [chainId, enabled, subscribe, unsubscribe])

  // Poll for updates from the context
  // This ensures we get updates when new events arrive
  useEffect(() => {
    if (!enabled) {
      return
    }

    const checkForUpdates = () => {
      const event = getLastEventForChain(chainId)
      if (event && event !== lastEvent) {
        setLastEvent(event)
      }
    }

    // Check immediately
    checkForUpdates()

    // Then poll every 100ms for updates
    // This is very lightweight and ensures reactive updates
    const interval = setInterval(checkForUpdates, 100)

    return () => clearInterval(interval)
  }, [chainId, enabled, getLastEventForChain, lastEvent])

  return {
    lastEvent,
    isConnected,
    error,
    reconnectAttempts,
    lastHeight: lastEvent?.height || null,
    lastTimestamp: lastEvent?.timestamp || null,
  }
}