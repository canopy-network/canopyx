import { useEffect, useRef, useState, useCallback } from 'react';

export interface BlockIndexedEvent {
  chainId: string;
  height: number;
  timestamp: string;
  block: {
    hash: string;
    time: string;
    proposerAddress: string;
  };
  summary: {
    height: number;
    height_time: string;
    num_txs: number;
  };
}

export interface ServerMessage {
  type: 'block.indexed' | 'subscribed' | 'unsubscribed' | 'ping' | 'error';
  payload: BlockIndexedEvent | { chainId?: string; message?: string; timestamp?: number };
}

interface UseWebSocketOptions {
  url: string;
  chainId?: string;  // Specific chain or undefined for all chains
  onBlockIndexed?: (event: BlockIndexedEvent) => void;
  onError?: (error: string) => void;
  reconnectInterval?: number;
  maxReconnectAttempts?: number;
  enabled?: boolean;  // Allow enabling/disabling the connection
}

interface UseWebSocketReturn {
  isConnected: boolean;
  lastEvent: BlockIndexedEvent | null;
  error: string | null;
  reconnectAttempts: number;
  subscribe: (chainId: string) => void;
  unsubscribe: (chainId: string) => void;
}

/**
 * Custom hook for managing WebSocket connection to Admin App for real-time block events.
 *
 * Features:
 * - Automatic reconnection with exponential backoff
 * - Server-side subscription management
 * - Connection status tracking
 * - Error handling
 * - Panic recovery on server prevents crashes
 *
 * @example
 * ```typescript
 * const { isConnected, lastEvent, subscribe } = useWebSocket({
 *   url: '/api/admin/ws',
 *   chainId: 'canopy_local',
 *   onBlockIndexed: (event) => {
 *     console.log('New block:', event.height);
 *     refetchData();
 *   }
 * });
 * ```
 */
export function useWebSocket({
  url,
  chainId,
  onBlockIndexed,
  onError,
  reconnectInterval = 3000,
  maxReconnectAttempts = 10,
  enabled = true,
}: UseWebSocketOptions): UseWebSocketReturn {
  const [isConnected, setIsConnected] = useState(false);
  const [lastEvent, setLastEvent] = useState<BlockIndexedEvent | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reconnectAttempts, setReconnectAttempts] = useState(0);

  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const shouldReconnectRef = useRef(true);
  const subscribedChainsRef = useRef<Set<string>>(new Set());
  const reconnectAttemptsRef = useRef(0);

  // Use refs for callbacks to avoid reconnection loops when they change
  const onBlockIndexedRef = useRef(onBlockIndexed);
  const onErrorRef = useRef(onError);

  useEffect(() => {
    onBlockIndexedRef.current = onBlockIndexed;
    onErrorRef.current = onError;
  }, [onBlockIndexed, onError]);

  const sendMessage = useCallback((message: { action: string; chainId: string }) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(message));
    }
  }, []);

  const subscribe = useCallback((targetChainId: string) => {
    subscribedChainsRef.current.add(targetChainId);
    sendMessage({ action: 'subscribe', chainId: targetChainId });
  }, [sendMessage]);

  const unsubscribe = useCallback((targetChainId: string) => {
    subscribedChainsRef.current.delete(targetChainId);
    sendMessage({ action: 'unsubscribe', chainId: targetChainId });
  }, [sendMessage]);

  const connect = useCallback(() => {
    if (!enabled) {
      return;
    }

    if (wsRef.current?.readyState === WebSocket.OPEN) {
      return;
    }

    try {
      console.log('[WebSocket] Connecting to:', url);

      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = () => {
        console.log('[WebSocket] Connected');
        setIsConnected(true);
        setError(null);
        reconnectAttemptsRef.current = 0;
        setReconnectAttempts(0);

        // Resubscribe to previously subscribed chains
        // Note: chainId from options is added to subscribedChainsRef by the caller
        subscribedChainsRef.current.forEach((cid) => {
          ws.send(JSON.stringify({ action: 'subscribe', chainId: cid }));
        });
      };

      ws.onmessage = (event) => {
        try {
          const message: ServerMessage = JSON.parse(event.data);

          switch (message.type) {
            case 'block.indexed': {
              const blockEvent = message.payload as BlockIndexedEvent;
              setLastEvent(blockEvent);
              onBlockIndexedRef.current?.(blockEvent);
              break;
            }
            case 'subscribed': {
              const payload = message.payload as { chainId?: string };
              console.log('[WebSocket] Subscribed to:', payload.chainId);
              break;
            }
            case 'unsubscribed': {
              const payload = message.payload as { chainId?: string };
              console.log('[WebSocket] Unsubscribed from:', payload.chainId);
              break;
            }
            case 'error': {
              const errorPayload = message.payload as { message?: string };
              const errorMsg = errorPayload.message || 'Unknown error';
              console.error('[WebSocket] Server error:', errorMsg);
              setError(errorMsg);
              onErrorRef.current?.(errorMsg);
              break;
            }
            case 'ping':
              // Keep-alive ping, no action needed
              break;
            default:
              console.warn('[WebSocket] Unknown message type:', message.type);
          }
        } catch (err) {
          console.error('[WebSocket] Failed to parse message:', err);
        }
      };

      ws.onerror = (event) => {
        console.error('[WebSocket] Error:', event);
        setError('WebSocket connection error');
      };

      ws.onclose = (event) => {
        console.log('[WebSocket] Disconnected:', event.code, event.reason);
        setIsConnected(false);
        wsRef.current = null;

        // Attempt reconnection if enabled and not at max attempts
        // Use ref for reconnectAttempts to avoid dependency loop
        const currentAttempts = reconnectAttemptsRef.current;
        if (shouldReconnectRef.current && currentAttempts < maxReconnectAttempts && enabled) {
          const delay = Math.min(reconnectInterval * Math.pow(1.5, currentAttempts), 30000);
          console.log(`[WebSocket] Reconnecting in ${delay}ms (attempt ${currentAttempts + 1}/${maxReconnectAttempts})`);

          reconnectTimeoutRef.current = setTimeout(() => {
            reconnectAttemptsRef.current += 1;
            setReconnectAttempts(reconnectAttemptsRef.current);
            connect();
          }, delay);
        } else if (currentAttempts >= maxReconnectAttempts) {
          setError('Max reconnection attempts reached');
          onErrorRef.current?.('Max reconnection attempts reached');
        }
      };
    } catch (err) {
      console.error('[WebSocket] Connection failed:', err);
      setError('Failed to establish WebSocket connection');
    }
  }, [url, enabled, maxReconnectAttempts, reconnectInterval]);

  const disconnect = useCallback(() => {
    shouldReconnectRef.current = false;

    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }

    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }

    setIsConnected(false);
  }, []);

  useEffect(() => {
    if (!enabled) {
      disconnect();
      return;
    }

    shouldReconnectRef.current = true;
    connect();

    return () => {
      disconnect();
    };
  }, [enabled, connect, disconnect]);

  return {
    isConnected,
    lastEvent,
    error,
    reconnectAttempts,
    subscribe,
    unsubscribe,
  };
}