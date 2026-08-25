import { useEffect, useRef, useState } from 'react'
import { useEventStream } from '../eventStream'

type ServerEvent = {
  type: string
  data: string
  id?: string
}

const MAX_LOG_ENTRIES = 100

function Home() {
  const { status, subscribe } = useEventStream()
  const [events, setEvents] = useState<ServerEvent[]>([])
  const nextId = useRef(0)

  useEffect(() => {
    return subscribe('heartbeat', (event) => {
      nextId.current += 1
      setEvents((prev) =>
        [{ type: event.type, data: event.data, id: String(nextId.current) }, ...prev].slice(0, MAX_LOG_ENTRIES),
      )
    })
  }, [subscribe])

  return (
    <>
      <div className="page-header">
        <h2>Live events</h2>
        <span className={`status status-${status}`}>{status === 'open' ? 'connected' : status}</span>
      </div>
      <p className="hint">
        Streamed from the CLI process over Server-Sent Events at <code>/api/events</code>.
      </p>
      <div className="panel panel-flush">
        <ul className="event-log">
          {events.length === 0 && <li className="event-empty">Waiting for events…</li>}
          {events.map((event) => (
            <li key={event.id}>
              <span className="event-type">{event.type}</span>
              <span className="event-data">{event.data}</span>
            </li>
          ))}
        </ul>
      </div>
    </>
  )
}

export default Home
