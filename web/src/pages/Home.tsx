import { BarChart3, Box, ClipboardCheck, Container, Database, Workflow } from 'lucide-react'
import { useEffect, useRef, useState, type ComponentType } from 'react'
import { Link } from 'react-router-dom'
import { listAgents, listDatasets, listEvaluations, listInstances, listModels, listTrainingRuns } from '../api'
import { useEventStream } from '../eventStream'
import Skeleton from '../Skeleton'

type ServerEvent = {
  type: string
  data: string
  id?: string
}

const MAX_LOG_ENTRIES = 50

interface DashboardCard {
  to: string
  label: string
  icon: ComponentType<{ size?: number; strokeWidth?: number }>
  value: string
}

function Home() {
  const { status, subscribe } = useEventStream()
  const [events, setEvents] = useState<ServerEvent[]>([])
  const nextId = useRef(0)
  const [cards, setCards] = useState<DashboardCard[] | null>(null)

  useEffect(() => {
    Promise.all([
      listModels().catch(() => []),
      listDatasets().catch(() => []),
      listTrainingRuns().catch(() => []),
      listAgents().catch(() => []),
      listInstances().catch(() => []),
      listEvaluations().catch(() => []),
    ]).then(([models, datasets, runs, agents, instances, evaluations]) => {
      const activeRuns = runs.filter((r) => r.status === 'running').length
      const runningInstances = instances.filter((i) => i.state === 'running').length
      setCards([
        { to: '/models', label: 'Models', icon: Box, value: String(models.length) },
        { to: '/datasets', label: 'Datasets', icon: Database, value: String(datasets.length) },
        { to: '/training', label: 'Training', icon: BarChart3, value: activeRuns > 0 ? `${activeRuns} running` : `${runs.length} runs` },
        { to: '/environments', label: 'Environments', icon: Container, value: `${runningInstances} running` },
        { to: '/agents', label: 'Agents', icon: Workflow, value: String(agents.length) },
        { to: '/evaluations', label: 'Evaluations', icon: ClipboardCheck, value: String(evaluations.length) },
      ])
    })
  }, [])

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
        <h2>Overview</h2>
        <span className={`status status-${status}`}>{status === 'open' ? 'connected' : status}</span>
      </div>

      <div className="dashboard-grid">
        {cards === null
          ? Array.from({ length: 6 }).map((_, i) => (
              <div className="dashboard-card" key={i}>
                <Skeleton width="1.2rem" height="1.2rem" />
                <Skeleton width="2.5rem" height="1.5rem" />
                <Skeleton width="60%" height="0.75rem" />
              </div>
            ))
          : cards.map((card) => {
              const Icon = card.icon
              return (
                <Link className="dashboard-card" to={card.to} key={card.to}>
                  <Icon size={18} strokeWidth={2} />
                  <div className="dashboard-card-value">{card.value}</div>
                  <div className="dashboard-card-label">{card.label}</div>
                </Link>
              )
            })}
      </div>

      <div className="page-header">
        <h3>Live activity</h3>
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
