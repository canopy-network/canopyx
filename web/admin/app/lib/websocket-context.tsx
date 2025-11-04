'use client'

import React, { createContext, useContext, useEffect, useState, useCallback, useRef } from 'react'
import { useWebSocket, BlockIndexedEvent } from '../hooks/useWebSocket'

interface WebSocketContextValue {
  // Connection state
  isConnected: boolean
  error: string | null
  reconnectAttempts: number

  // Event state
  lastBlockIndexed: BlockIndexedEvent | null
  getLastEventForChain: (chainId: string) => BlockIndexedEvent | null
  chainEventsMap: Map<string, BlockIndexedEvent> // Expose the map for iteration

  // Subscription management
  subscribe: (chainId: string) => void
  unsubscribe: (chainId: string) => void

  // Internal state
  activeSubscriptions: Set<string>
}

const WebSocketContext = createContext<WebSocketContextValue | null>(null)

interface WebSocketProviderProps {
  children: React.ReactNode
}

/**
 * WebSocket Context Provider
 *
 * Manages a single WebSocket connection for the entire application.
 * Provides reactive event updates and subscription management.
 *
 * Features:
 * - Single shared connection across all components
 * - Lazy connection (only connects when first component subscribes)
 * - Automatic reconnection with exponential backoff
 * - Per-chain event tracking
 * - Connection pooling (keeps connection alive after last unsubscribe)
 * - Uses same-origin proxy pattern (/api/admin/ws) - no environment variables needed
 *
 * @example
 * ```tsx
 * <WebSocketProvider>
 *   <App />
 * </WebSocketProvider>
 * ```
 */
export function WebSocketProvider({ children }: WebSocketProviderProps) {
  // Use same-origin WebSocket connection
  // The nginx proxy routes /api/admin/ws to Admin service internally
  // Development: ws://localhost:3003/api/admin/ws (via nginx proxy)
  // Production: wss://yourdomain.com/api/admin/ws (via ingress)
  const wsUrl = typeof window !== 'undefined'
    ? `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/admin/ws`
    : ''

  // Track active subscriptions from all components
  const [activeSubscriptions, setActiveSubscriptions] = useState<Set<string>>(new Set())

  // Track last event per chain
  const [chainEvents, setChainEvents] = useState<Map<string, BlockIndexedEvent>>(new Map())

  // Connection keepalive timeout
  const keepaliveTimeoutRef = useRef<NodeJS.Timeout | null>(null)

  // Determine if we should be connected
  const shouldConnect = activeSubscriptions.size > 0

  // Handle incoming block events
  const handleBlockIndexed = useCallback((event: BlockIndexedEvent) => {
    setChainEvents((prev) => {
      const next = new Map(prev)
      next.set(event.chainId, event)
      return next
    })
  }, [])

  // Handle errors
  const handleError = useCallback((error: string) => {
    console.error('[WebSocket Context] Error:', error)
  }, [])

  // Use the underlying WebSocket hook
  const {
    isConnected,
    lastEvent,
    error,
    reconnectAttempts,
    subscribe: wsSubscribe,
    unsubscribe: wsUnsubscribe,
  } = useWebSocket({
    url: wsUrl,
    onBlockIndexed: handleBlockIndexed,
    onError: handleError,
    enabled: shouldConnect,
    reconnectInterval: 3000,
    maxReconnectAttempts: 10,
  })

  // Subscribe to a chain
  const subscribe = useCallback((chainId: string) => {
    setActiveSubscriptions((prev) => {
      const next = new Set(prev)
      if (!next.has(chainId)) {
        next.add(chainId)
        // Clear any pending keepalive timeout since we have active subscribers
        if (keepaliveTimeoutRef.current) {
          clearTimeout(keepaliveTimeoutRef.current)
          keepaliveTimeoutRef.current = null
        }
        // Subscribe on the WebSocket
        wsSubscribe(chainId)
      }
      return next
    })
  }, [wsSubscribe])

  // Unsubscribe from a chain
  const unsubscribe = useCallback((chainId: string) => {
    setActiveSubscriptions((prev) => {
      const next = new Set(prev)
      if (next.has(chainId)) {
        next.delete(chainId)
        // Unsubscribe on the WebSocket
        wsUnsubscribe(chainId)

        // If this was the last subscription, set a keepalive timer
        // Keep the connection alive for 30 seconds in case user navigates back
        if (next.size === 0) {
          if (keepaliveTimeoutRef.current) {
            clearTimeout(keepaliveTimeoutRef.current)
          }
          keepaliveTimeoutRef.current = setTimeout(() => {
            console.log('[WebSocket Context] Keepalive expired, connection will close')
            // Connection will automatically close due to enabled=false
          }, 30000)
        }
      }
      return next
    })
  }, [wsUnsubscribe])

  // Get last event for a specific chain
  const getLastEventForChain = useCallback((chainId: string): BlockIndexedEvent | null => {
    return chainEvents.get(chainId) || null
  }, [chainEvents])

  // Cleanup keepalive timeout on unmount
  useEffect(() => {
    return () => {
      if (keepaliveTimeoutRef.current) {
        clearTimeout(keepaliveTimeoutRef.current)
      }
    }
  }, [])

  const value: WebSocketContextValue = {
    isConnected,
    error,
    reconnectAttempts,
    lastBlockIndexed: lastEvent,
    getLastEventForChain,
    chainEventsMap: chainEvents,
    subscribe,
    unsubscribe,
    activeSubscriptions,
  }

  return (
    <WebSocketContext.Provider value={value}>
      {children}
    </WebSocketContext.Provider>
  )
}

/**
 * Hook to access WebSocket context
 *
 * @throws Error if used outside WebSocketProvider
 *
 * @example
 * ```tsx
 * const { isConnected, subscribe, unsubscribe } = useWebSocketContext()
 * ```
 */
export function useWebSocketContext() {
  const context = useContext(WebSocketContext)

  if (!context) {
    throw new Error('useWebSocketContext must be used within a WebSocketProvider')
  }

  return context
}