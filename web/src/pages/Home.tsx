import {
  BarChart3,
  BookOpen,
  Box,
  ClipboardCheck,
  Container,
  Database,
  Rocket,
  Trophy,
  Workflow,
  Wrench,
} from 'lucide-react'
import { useEffect, useRef, useState, type ComponentType } from 'react'
import { Link } from 'react-router-dom'
import {
  listAgents,
  listBenchmarks,
  listDatasets,
  listDeployments,
  listEvaluations,
  listKnowledgeBases,
  listModels,
  listTools,
  listTrainingRuns,
  listWorkspaces,
} from '../api'
import { useEventStream } from '../eventStream'
import Skeleton from '../Skeleton'

interface ActivityEntry {
  id: string
  label: string
  detail: string
  at: string
}

const MAX_LOG_ENTRIES = 50

// Real activity events published on the CLI's eventbus. `heartbeat` is
// deliberately excluded — it carries no activity, and the connection pill
// already shows the stream is alive.
const ACTIVITY_EVENTS = [
  'training.status',
  'training.progress',
  'agent.step',
  'agent.message',
  'evaluation.status',
  'evaluation.progress',
  'benchmark.status',
  'benchmark.progress',
  'workspace.exec.output',
  'workspace.exec.status',
] as const

const EVENT_LABELS: Record<string, string> = {
  'training.status': 'training',
  'training.progress': 'training',
  'agent.step': 'agent',
  'agent.message': 'agent',
  'evaluation.status': 'evaluation',
  'evaluation.progress': 'evaluation',
  'benchmark.status': 'benchmark',
  'benchmark.progress': 'benchmark',
  'workspace.exec.output': 'workspace',
  'workspace.exec.status': 'workspace',
}

interface DashboardCard {
  to: string
  label: string
  icon: ComponentType<{ size?: number; strokeWidth?: number }>
  value: string
}

function truncate(text: string, max = 120): string {
  const clean = text.replace(/\s+/g, ' ').trim()
  return clean.length > max ? `${clean.slice(0, max)}…` : clean
}

function describeEvent(type: string, raw: string): string {
  let d: Record<string, unknown>
  try {
    d = JSON.parse(raw) as Record<string, unknown>
  } catch {
    return truncate(raw)
  }
  const s = (k: string): string => (typeof d[k] === 'string' ? (d[k] as string) : '')
  const n = (k: string): number | null => (typeof d[k] === 'number' ? (d[k] as number) : null)

  switch (type) {
    case 'training.status':
      return `${s('modelName') || s('id') || 'run'} — ${s('status')}${s('error') ? `: ${truncate(s('error'), 80)}` : ''}`
    case 'training.progress': {
      const loss = n('trainLoss')
      return `${s('runId') || 'run'} — iter ${n('iteration') ?? '?'}${loss != null ? `, loss ${loss.toFixed(3)}` : ''}`
    }
    case 'agent.step':
      return `${s('nodeType') || 'node'} — ${s('phase') || 'done'}${s('command') ? `: ${truncate(s('command'), 80)}` : ''}`
    case 'agent.message':
      return `${s('kind') || 'message'}: ${truncate(s('content'))}`
    case 'evaluation.status':
      return `${s('evaluationName') || s('id') || 'run'} — ${s('status')}`
    case 'benchmark.status':
      return `${s('benchmarkName') || s('id') || 'run'} — ${s('status')}`
    case 'evaluation.progress':
      return `${s('agentName') || 'agent'} — ${d['passed'] ? 'passed' : 'failed'} a test case`
    case 'benchmark.progress':
      return `${s('modelName') || 'model'} — ${d['passed'] ? 'passed' : 'failed'} a test case`
    case 'workspace.exec.output':
      return truncate(s('chunk') || raw)
    case 'workspace.exec.status':
      return `exec ${s('id')} — ${s('status')}`
    default:
      return truncate(raw)
  }
}

function Home() {
  const { status, subscribe } = useEventStream()
  const [activity, setActivity] = useState<ActivityEntry[]>([])
  const nextId = useRef(0)
  const [cards, setCards] = useState<DashboardCard[] | null>(null)

  useEffect(() => {
    Promise.all([
      listModels().catch(() => []),
      listDatasets().catch(() => []),
      listTrainingRuns().catch(() => []),
      listBenchmarks().catch(() => []),
      listWorkspaces().catch(() => []),
      listKnowledgeBases().catch(() => []),
      listTools().catch(() => []),
      listAgents().catch(() => []),
      listEvaluations().catch(() => []),
      listDeployments().catch(() => []),
    ]).then(([models, datasets, runs, benchmarks, workspaces, knowledge, tools, agents, evaluations, deployments]) => {
      const activeRuns = runs.filter((r) => r.status === 'running').length
      setCards([
        { to: '/models', label: 'Models', icon: Box, value: String(models.length) },
        { to: '/datasets', label: 'Datasets', icon: Database, value: String(datasets.length) },
        {
          to: '/training',
          label: 'Training',
          icon: BarChart3,
          value: activeRuns > 0 ? `${activeRuns} running` : `${runs.length} runs`,
        },
        { to: '/benchmarks', label: 'Benchmarks', icon: Trophy, value: String(benchmarks.length) },
        { to: '/workspaces', label: 'Workspaces', icon: Container, value: String(workspaces.length) },
        { to: '/knowledge', label: 'Knowledge', icon: BookOpen, value: String(knowledge.length) },
        { to: '/tools', label: 'Tools', icon: Wrench, value: String(tools.length) },
        { to: '/agents', label: 'Agents', icon: Workflow, value: String(agents.length) },
        { to: '/evaluations', label: 'Evaluations', icon: ClipboardCheck, value: String(evaluations.length) },
        { to: '/deployments', label: 'Deployments', icon: Rocket, value: String(deployments.length) },
      ])
    })
  }, [])

  useEffect(() => {
    const unsubscribes = ACTIVITY_EVENTS.map((type) =>
      subscribe(type, (event) => {
        nextId.current += 1
        setActivity((prev) =>
          [
            {
              id: String(nextId.current),
              label: EVENT_LABELS[event.type] ?? event.type,
              detail: describeEvent(event.type, event.data),
              at: new Date().toLocaleTimeString(),
            },
            ...prev,
          ].slice(0, MAX_LOG_ENTRIES),
        )
      }),
    )
    return () => unsubscribes.forEach((fn) => fn())
  }, [subscribe])

  return (
    <>
      <div className="page-header">
        <h2>Overview</h2>
        <span className={`status status-${status}`}>{status === 'open' ? 'connected' : status}</span>
      </div>

      <div className="dashboard-grid">
        {cards === null
          ? Array.from({ length: 10 }).map((_, i) => (
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
        Training, agent, evaluation, benchmark and workspace events streamed from the CLI over Server-Sent Events at{' '}
        <code>/api/events</code>.
      </p>
      <div className="panel panel-flush">
        <ul className="event-log">
          {activity.length === 0 && <li className="event-empty">No activity yet — start a training run or an agent.</li>}
          {activity.map((entry) => (
            <li key={entry.id}>
              <span className="event-type">{entry.label}</span>
              <span className="event-data">{entry.detail}</span>
              <span className="event-time">{entry.at}</span>
            </li>
          ))}
        </ul>
      </div>
    </>
  )
}

export default Home
