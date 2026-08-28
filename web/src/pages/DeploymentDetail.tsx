import { Copy, Play, Square } from 'lucide-react'
import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  getDeployment,
  sendDeploymentMessage,
  startDeployment,
  stopDeploymentSession,
  type AgentMessageEvent,
  type AgentStepEvent,
  type ChatMessage,
  type Deployment,
  type DeploymentSession,
} from '../api'
import { useEventStream } from '../eventStream'
import { formatDateTime } from '../lib/format'
import { useToast } from '../Toast'

type ChatEntry = ChatMessage & { kind?: 'progress' | 'final' }

function DeploymentDetail() {
  const { name = '' } = useParams<{ name: string }>()
  const showToast = useToast()
  const { subscribe } = useEventStream()

  const [deployment, setDeployment] = useState<Deployment | null>(null)
  const [error, setError] = useState<string | null>(null)

  const [session, setSession] = useState<DeploymentSession | null>(null)
  const [starting, setStarting] = useState(false)
  const [messages, setMessages] = useState<ChatEntry[]>([])
  const [steps, setSteps] = useState<AgentStepEvent[]>([])
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const streamedFinalRef = useRef(false)
  const endRef = useRef<HTMLDivElement>(null)

  const reload = useCallback(() => {
    getDeployment(name).then(setDeployment).catch((err: Error) => setError(err.message))
  }, [name])

  useEffect(() => {
    reload()
  }, [reload])

  // Stop the session when leaving the page.
  const sessionRef = useRef<DeploymentSession | null>(null)
  useEffect(() => {
    sessionRef.current = session
  }, [session])
  useEffect(() => {
    return () => {
      if (sessionRef.current) void stopDeploymentSession(sessionRef.current.id).catch(() => {})
    }
  }, [])

  const runId = session?.runId ?? null

  useEffect(() => {
    return subscribe('agent.step', (event) => {
      const step = JSON.parse(event.data) as AgentStepEvent
      if (runId && step.runId === runId && step.phase !== 'start') setSteps((prev) => [...prev, step])
    })
  }, [subscribe, runId])

  useEffect(() => {
    return subscribe('agent.message', (event) => {
      const msg = JSON.parse(event.data) as AgentMessageEvent
      if (!runId || msg.runId !== runId) return
      if (msg.kind === 'final') streamedFinalRef.current = true
      setMessages((prev) => [...prev, { role: 'assistant', content: msg.content, timestamp: new Date().toISOString(), kind: msg.kind }])
    })
  }, [subscribe, runId])

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, steps])

  const handleStart = async () => {
    setStarting(true)
    setError(null)
    try {
      const s = await startDeployment(name)
      setSession(s)
      setMessages([])
      setSteps([])
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setStarting(false)
    }
  }

  const handleStop = async () => {
    if (!session) return
    await stopDeploymentSession(session.id).catch(() => {})
    setSession(null)
    setMessages([])
    setSteps([])
  }

  const handleSend = async (e: FormEvent) => {
    e.preventDefault()
    if (!session || !input.trim()) return
    const userMsg: ChatEntry = { role: 'user', content: input.trim(), timestamp: new Date().toISOString() }
    setMessages((prev) => [...prev, userMsg])
    setInput('')
    setSending(true)
    setSteps([])
    streamedFinalRef.current = false
    try {
      const reply = await sendDeploymentMessage(session.id, userMsg.content)
      if (!streamedFinalRef.current) setMessages((prev) => [...prev, reply])
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSending(false)
    }
  }

  const copyPath = () => {
    if (!session) return
    navigator.clipboard?.writeText(session.workspacePath).then(
      () => showToast('Path copied'),
      () => showToast('Could not copy'),
    )
  }

  return (
    <>
      <div className="page-header">
        <h2>
          <Link to="/deployments">Deployments</Link> / {name}
        </h2>
      </div>

      {error && <p className="error">{error}</p>}

      {deployment && (
        <section className="panel">
          <h3>Deployment info</h3>
          <dl className="info-list">
            <dt>Agent</dt>
            <dd>
              <Link to={`/agents/${encodeURIComponent(deployment.agentName)}`}>{deployment.agentName}</Link>
            </dd>
            <dt>Real workspace</dt>
            <dd>
              <Link to={`/workspaces/${encodeURIComponent(deployment.workspaceName)}`}>{deployment.workspaceName}</Link>
            </dd>
            <dt>Created</dt>
            <dd>{formatDateTime(deployment.createdAt)}</dd>
          </dl>

          {!session ? (
            <button type="button" onClick={handleStart} disabled={starting}>
              <Play size={14} /> {starting ? 'Starting…' : 'Start'}
            </button>
          ) : (
            <div className="row-actions">
              <span className="status status-open">running</span>
              <button type="button" onClick={handleStop}>
                <Square size={14} /> Stop
              </button>
            </div>
          )}

          {session && (
            <p className="hint">
              Working directory: <code>{session.workspacePath}</code>{' '}
              <button type="button" className="icon-button" title="Copy path" aria-label="Copy path" onClick={copyPath}>
                <Copy size={14} />
              </button>{' '}
              — the agent's changes here persist.
            </p>
          )}
        </section>
      )}

      {session && (
        <section className="panel">
          <h3>Chat</h3>
          <div className="chat-log">
            {messages.length === 0 && <p className="hint">Ask the agent to do some work.</p>}
            {messages.map((m, i) => (
              <div
                key={i}
                className={`chat-message chat-message-${m.role} ${m.kind === 'progress' ? 'chat-message-progress' : ''}`}
              >
                <span>{m.content}</span>
              </div>
            ))}
            <div ref={endRef} />
          </div>

          <form className="inline-form" onSubmit={handleSend}>
            <input
              type="text"
              placeholder="Type a message…"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              disabled={sending}
              autoFocus
            />
            <button type="submit" disabled={sending || !input.trim()}>
              {sending ? 'Working…' : 'Send'}
            </button>
          </form>

          {steps.length > 0 && (
            <>
              <h3>Live steps</h3>
              <ul className="event-log">
                {steps.map((s, i) => (
                  <li key={i}>
                    <span className="event-type">{s.nodeType}</span>
                    <span className="event-data">{s.output}</span>
                  </li>
                ))}
              </ul>
            </>
          )}
        </section>
      )}
    </>
  )
}

export default DeploymentDetail
