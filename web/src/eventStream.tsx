import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

export type ConnectionState = 'connecting' | 'open' | 'closed'
type Listener = (event: MessageEvent<string>) => void

interface EventStreamContextValue {
  status: ConnectionState
  subscribe: (type: string, listener: Listener) => () => void
}

const EventStreamContext = createContext<EventStreamContextValue | null>(null)

// EventStreamProvider owns the single EventSource connection to /api/events
// for the whole app, so pages subscribe to it instead of each opening their
// own. The source is created during the initial render (not in an effect)
// so it already exists by the time a child's own mount effect calls
// subscribe().
export function EventStreamProvider({ children }: { children: ReactNode }) {
  const [source] = useState(() => new EventSource('/api/events'))
  const [status, setStatus] = useState<ConnectionState>('connecting')

  useEffect(() => {
    const handleOpen = () => setStatus('open')
    const handleError = () => setStatus('closed')
    source.addEventListener('open', handleOpen)
    source.addEventListener('error', handleError)
    return () => {
      source.removeEventListener('open', handleOpen)
      source.removeEventListener('error', handleError)
      source.close()
    }
  }, [source])

  const subscribe = useCallback(
    (type: string, listener: Listener) => {
      source.addEventListener(type, listener)
      return () => source.removeEventListener(type, listener)
    },
    [source],
  )

  const value = useMemo(() => ({ status, subscribe }), [status, subscribe])

  return <EventStreamContext.Provider value={value}>{children}</EventStreamContext.Provider>
}

export function useEventStream(): EventStreamContextValue {
  const ctx = useContext(EventStreamContext)
  if (!ctx) throw new Error('useEventStream must be used within an EventStreamProvider')
  return ctx
}
