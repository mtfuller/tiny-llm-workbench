import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'

export type ConnectionState = 'connecting' | 'open' | 'reconnecting'
type Listener = (event: MessageEvent<string>) => void

interface EventStreamContextValue {
  status: ConnectionState
  /** Force an immediate reconnect, resetting the backoff. */
  reconnect: () => void
  subscribe: (type: string, listener: Listener) => () => void
}

const EventStreamContext = createContext<EventStreamContextValue | null>(null)

const MAX_BACKOFF_MS = 30_000

// EventStreamProvider owns the single EventSource connection to /api/events
// for the whole app. Pages subscribe through it rather than each opening
// their own connection.
//
// The browser's native EventSource retries on its own, but silently and with
// no signal the UI can show. This provider drives an explicit capped
// exponential backoff instead: on a drop it closes the socket, flips status
// to 'reconnecting' (so a banner can appear), and schedules a fresh connect.
// Subscriptions are tracked in a registry and re-attached to each new socket,
// so a reconnect is transparent to consumers and to any long-running
// training / eval run they're watching.
export function EventStreamProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<ConnectionState>('connecting')

  const sourceRef = useRef<EventSource | null>(null)
  const listenersRef = useRef<Map<string, Set<Listener>>>(new Map())
  const attemptRef = useRef(0)
  const timerRef = useRef<number | null>(null)
  const connectRef = useRef<() => void>(() => {})

  const clearTimer = () => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }

  const connect = useCallback(() => {
    clearTimer()
    sourceRef.current?.close()

    const es = new EventSource('/api/events')
    sourceRef.current = es

    // Re-attach every live subscription to the new socket.
    for (const [type, set] of listenersRef.current) {
      for (const listener of set) es.addEventListener(type, listener)
    }

    es.addEventListener('open', () => {
      attemptRef.current = 0
      setStatus('open')
    })

    es.addEventListener('error', () => {
      es.close()
      setStatus('reconnecting')
      if (timerRef.current !== null) return
      const delay = Math.min(1000 * 2 ** attemptRef.current, MAX_BACKOFF_MS)
      attemptRef.current += 1
      timerRef.current = window.setTimeout(() => {
        timerRef.current = null
        connectRef.current()
      }, delay)
    })
  }, [])

  // Keep the ref pointing at connect so the backoff timer (scheduled from
  // inside connect's own error handler) can call the latest one without a
  // definition-order cycle. connect is referentially stable, so this runs once.
  useEffect(() => {
    connectRef.current = connect
  }, [connect])

  const reconnect = useCallback(() => {
    clearTimer()
    attemptRef.current = 0
    setStatus('connecting')
    connect()
  }, [connect])

  useEffect(() => {
    connect()
    return () => {
      clearTimer()
      sourceRef.current?.close()
      sourceRef.current = null
    }
  }, [connect])

  const subscribe = useCallback((type: string, listener: Listener) => {
    let set = listenersRef.current.get(type)
    if (!set) {
      set = new Set()
      listenersRef.current.set(type, set)
    }
    set.add(listener)
    sourceRef.current?.addEventListener(type, listener)

    return () => {
      set?.delete(listener)
      sourceRef.current?.removeEventListener(type, listener)
    }
  }, [])

  const value = useMemo(() => ({ status, reconnect, subscribe }), [status, reconnect, subscribe])

  return <EventStreamContext.Provider value={value}>{children}</EventStreamContext.Provider>
}

export function useEventStream(): EventStreamContextValue {
  const ctx = useContext(EventStreamContext)
  if (!ctx) throw new Error('useEventStream must be used within an EventStreamProvider')
  return ctx
}
