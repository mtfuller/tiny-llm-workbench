import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  getEvaluation,
  listAgents,
  listEvaluationRuns,
  startEvaluationRun,
  type Agent,
  type Evaluation,
  type EvaluationRun,
} from '../api'
import { useEventStream } from '../eventStream'

function formatDuration(startedAt: string, finishedAt?: string): string {
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now()
  const seconds = Math.max(0, Math.round((end - new Date(startedAt).getTime()) / 1000))
  if (seconds < 60) return `${seconds}s`
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}

function EvaluationDetail() {
  const { name = '' } = useParams<{ name: string }>()
  const { subscribe } = useEventStream()

  const [evaluation, setEvaluation] = useState<Evaluation | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [selectedAgents, setSelectedAgents] = useState<string[]>([])
  const [runs, setRuns] = useState<EvaluationRun[]>([])
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getEvaluation(name)
      .then(setEvaluation)
      .catch((err: Error) => setError(err.message))
    listAgents()
      .then(setAgents)
      .catch(() => setAgents([]))
    listEvaluationRuns()
      .then((all) => setRuns(all.filter((r) => r.evaluationName === name)))
      .catch(() => setRuns([]))
  }, [name])

  useEffect(() => {
    const unsubscribeStatus = subscribe('evaluation.status', (event) => {
      const run = JSON.parse(event.data) as EvaluationRun
      if (run.evaluationName !== name) return
      setRuns((prev) => {
        const others = prev.filter((r) => r.id !== run.id)
        return [run, ...others]
      })
    })
    return unsubscribeStatus
  }, [subscribe, name])

  const activeRun = useMemo(() => runs.find((r) => r.status === 'running'), [runs])

  // SSE can race a fast-finishing run (same lesson learned building
  // Training); poll while a run for this evaluation is active so the UI
  // always reconciles.
  useEffect(() => {
    if (!activeRun) return
    const interval = setInterval(() => {
      listEvaluationRuns()
        .then((all) => setRuns(all.filter((r) => r.evaluationName === name)))
        .catch(() => {})
    }, 3000)
    return () => clearInterval(interval)
  }, [activeRun, name])

  const toggleAgent = (agentName: string) => {
    setSelectedAgents((prev) => (prev.includes(agentName) ? prev.filter((a) => a !== agentName) : [...prev, agentName]))
  }

  const handleRun = async () => {
    if (selectedAgents.length === 0) return
    setStarting(true)
    setError(null)
    try {
      const run = await startEvaluationRun(name, selectedAgents)
      setRuns((prev) => [run, ...prev])
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setStarting(false)
    }
  }

  const latestRun = runs[0]

  return (
    <>
      <div className="page-header">
        <h2>
          <Link to="/evaluations">Evaluations</Link> / {name}
        </h2>
      </div>

      {error && <p className="error">{error}</p>}

      {evaluation && (
        <section className="panel">
          <h3>Test cases</h3>
          {evaluation.environment && (
            <p className="hint">
              Launches environment <code>{evaluation.environment}</code> for the run (agents can't act
              on it yet — see the roadmap notes).
            </p>
          )}
          <table className="data-table">
            <thead>
              <tr>
                <th>Prompt</th>
                <th>Assertions</th>
              </tr>
            </thead>
            <tbody>
              {evaluation.testCases.map((tc) => (
                <tr key={tc.id}>
                  <td>{tc.prompt}</td>
                  <td>
                    {tc.assertions.map((a, i) => (
                      <div key={i}>
                        <code>
                          {a.type} "{a.value}"
                        </code>
                      </div>
                    ))}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      <section className="panel">
        <h3>Run against</h3>
        {agents.length === 0 && <p className="empty-state">No agents available. Create one on the Agents page.</p>}
        {agents.length > 0 && (
          <div className="agent-checklist">
            {agents.map((a) => (
              <label key={a.name} className="checkbox-label">
                <input type="checkbox" checked={selectedAgents.includes(a.name)} onChange={() => toggleAgent(a.name)} />
                {a.name}
              </label>
            ))}
          </div>
        )}
        <button type="button" onClick={handleRun} disabled={starting || !!activeRun || selectedAgents.length === 0}>
          {activeRun ? 'A run is already in progress' : starting ? 'Starting…' : 'Run evaluation'}
        </button>
      </section>

      {latestRun && (
        <section className="panel">
          <div className="page-header">
            <h3>Results</h3>
            <span className={`status ${latestRun.status === 'failed' ? 'status-closed' : latestRun.status === 'succeeded' ? 'status-open' : 'status-connecting'}`}>
              {latestRun.status}
            </span>
          </div>
          {latestRun.error && <p className="error">{latestRun.error}</p>}
          {latestRun.agentResults.length === 0 && <p className="hint">Running…</p>}
          {latestRun.agentResults.length > 0 && evaluation && (
            <div className="panel panel-flush">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Agent</th>
                    {evaluation.testCases.map((tc, i) => (
                      <th key={tc.id} title={tc.prompt}>
                        Test {i + 1}
                      </th>
                    ))}
                    <th>Score</th>
                  </tr>
                </thead>
                <tbody>
                  {latestRun.agentResults.map((ar) => (
                    <tr key={ar.agentName}>
                      <td>{ar.agentName}</td>
                      {evaluation.testCases.map((tc) => {
                        const result = ar.results.find((r) => r.testCaseId === tc.id)
                        return (
                          <td key={tc.id} title={result?.reply || result?.error || ''}>
                            {result ? (
                              <span className={`badge ${result.passed ? 'badge-purple' : ''}`}>
                                {result.passed ? 'pass' : 'fail'}
                              </span>
                            ) : (
                              '—'
                            )}
                          </td>
                        )
                      })}
                      <td>
                        {ar.passed}/{ar.total}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="hint">Duration: {formatDuration(latestRun.startedAt, latestRun.finishedAt)}</p>
        </section>
      )}

      {runs.length > 1 && (
        <section className="panel">
          <h3>Run history</h3>
          <ul className="event-log">
            {runs.slice(1).map((r) => (
              <li key={r.id}>
                <span className="event-type">{r.status}</span>
                <span className="event-data">
                  {r.agentNames.join(', ')} — {formatDuration(r.startedAt, r.finishedAt)}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  )
}

export default EvaluationDetail
