import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  getEvaluation,
  listAgents,
  listEnvironments,
  listEvaluationRuns,
  saveEvaluation,
  startEvaluationRun,
  type Agent,
  type Environment,
  type Evaluation,
  type EvaluationRun,
  type TestCase,
  type TestCaseResult,
} from '../api'
import { useEventStream } from '../eventStream'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { formatAssertion, TestCaseFields, toDraftTestCases, toPayloadTestCases, type DraftTestCase } from '../TestCaseEditor'
import { usePagination } from '../usePagination'

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
  const [environments, setEnvironments] = useState<Environment[]>([])
  const [selectedAgents, setSelectedAgents] = useState<string[]>([])
  const [runs, setRuns] = useState<EvaluationRun[]>([])
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [editing, setEditing] = useState(false)
  const [editEnvironment, setEditEnvironment] = useState('')
  const [editTestCases, setEditTestCases] = useState<DraftTestCase[]>([])
  const [savingEdit, setSavingEdit] = useState(false)

  const [selectedResult, setSelectedResult] = useState<{
    agentName: string
    testCase: TestCase
    result: TestCaseResult
  } | null>(null)

  const reloadEvaluation = useCallback(() => {
    getEvaluation(name)
      .then(setEvaluation)
      .catch((err: Error) => setError(err.message))
  }, [name])

  useEffect(() => {
    reloadEvaluation()
    listAgents()
      .then(setAgents)
      .catch(() => setAgents([]))
    listEnvironments()
      .then(setEnvironments)
      .catch(() => setEnvironments([]))
    listEvaluationRuns()
      .then((all) => setRuns(all.filter((r) => r.evaluationName === name)))
      .catch(() => setRuns([]))
  }, [name, reloadEvaluation])

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

  const startEditing = () => {
    if (!evaluation) return
    setEditEnvironment(evaluation.environment ?? '')
    setEditTestCases(toDraftTestCases(evaluation.testCases))
    setEditing(true)
  }

  const cancelEditing = () => setEditing(false)

  const handleSaveEdit = async (e: React.FormEvent) => {
    e.preventDefault()

    const payloadTestCases = toPayloadTestCases(editTestCases)
    if (payloadTestCases.length === 0) {
      setError('At least one test case with a prompt is required.')
      return
    }

    setSavingEdit(true)
    setError(null)
    try {
      await saveEvaluation({ name, environment: editEnvironment || undefined, testCases: payloadTestCases })
      setEditing(false)
      reloadEvaluation()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSavingEdit(false)
    }
  }

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
  const pastRuns = runs.slice(1)

  const {
    page: testCasePage,
    setPage: setTestCasePage,
    pageCount: testCasePageCount,
    pageItems: testCasePageItems,
  } = usePagination(evaluation?.testCases ?? [])

  const {
    page: agentResultPage,
    setPage: setAgentResultPage,
    pageCount: agentResultPageCount,
    pageItems: agentResultPageItems,
  } = usePagination(latestRun?.agentResults ?? [])

  const {
    page: pastRunPage,
    setPage: setPastRunPage,
    pageCount: pastRunPageCount,
    pageItems: pastRunPageItems,
  } = usePagination(pastRuns)

  return (
    <>
      <div className="page-header">
        <h2>
          <Link to="/evaluations">Evaluations</Link> / {name}
        </h2>
      </div>

      {error && <p className="error">{error}</p>}

      {evaluation && !editing && (
        <section className="panel">
          <div className="page-header">
            <h3>Test cases</h3>
            <button type="button" onClick={startEditing}>
              Edit
            </button>
          </div>
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
              {testCasePageItems.map((tc) => (
                <tr key={tc.id}>
                  <td>{tc.prompt}</td>
                  <td>
                    {tc.assertions.map((a, i) => (
                      <div key={i}>
                        <code title={a.type === 'json_schema' ? a.value : undefined}>{formatAssertion(a)}</code>
                      </div>
                    ))}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pagination
            page={testCasePage}
            pageCount={testCasePageCount}
            onChange={setTestCasePage}
            shownCount={evaluation.testCases.length}
            totalCount={evaluation.testCases.length}
            itemLabel="test cases"
          />
        </section>
      )}

      {evaluation && editing && (
        <section className="panel">
          <h3>Edit test cases</h3>
          <form className="stacked-form" onSubmit={handleSaveEdit}>
            <label>
              Environment (optional)
              <select value={editEnvironment} onChange={(e) => setEditEnvironment(e.target.value)}>
                <option value="">None</option>
                {environments.map((env) => (
                  <option key={env.name} value={env.name}>
                    {env.name}
                  </option>
                ))}
              </select>
            </label>

            <TestCaseFields testCases={editTestCases} onChange={setEditTestCases} />

            <div className="row-actions">
              <button type="submit" disabled={savingEdit}>
                {savingEdit ? 'Saving…' : 'Save changes'}
              </button>
              <button type="button" onClick={cancelEditing}>
                Cancel
              </button>
            </div>
          </form>
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
              <div className="table-scroll">
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
                    {agentResultPageItems.map((ar) => (
                      <tr key={ar.agentName}>
                        <td>{ar.agentName}</td>
                        {evaluation.testCases.map((tc) => {
                          const result = ar.results.find((r) => r.testCaseId === tc.id)
                          return (
                            <td key={tc.id}>
                              {result ? (
                                <button
                                  type="button"
                                  className="result-cell"
                                  onClick={() => setSelectedResult({ agentName: ar.agentName, testCase: tc, result })}
                                >
                                  <span className={`badge ${result.passed ? 'badge-purple' : ''}`}>
                                    {result.passed ? 'pass' : 'fail'}
                                  </span>
                                </button>
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
              <Pagination
                page={agentResultPage}
                pageCount={agentResultPageCount}
                onChange={setAgentResultPage}
                shownCount={latestRun.agentResults.length}
                totalCount={latestRun.agentResults.length}
                itemLabel="agents"
              />
            </div>
          )}
          <p className="hint">Duration: {formatDuration(latestRun.startedAt, latestRun.finishedAt)}</p>
        </section>
      )}

      {pastRuns.length > 0 && (
        <section className="panel">
          <h3>Run history</h3>
          <ul className="event-log">
            {pastRunPageItems.map((r) => (
              <li key={r.id}>
                <span className="event-type">{r.status}</span>
                <span className="event-data">
                  {r.agentNames.join(', ')} — {formatDuration(r.startedAt, r.finishedAt)}
                </span>
              </li>
            ))}
          </ul>
          <Pagination
            page={pastRunPage}
            pageCount={pastRunPageCount}
            onChange={setPastRunPage}
            shownCount={pastRuns.length}
            totalCount={pastRuns.length}
            itemLabel="past runs"
          />
        </section>
      )}

      {selectedResult && (
        <Modal
          title={`${selectedResult.agentName} — ${selectedResult.result.passed ? 'pass' : 'fail'}`}
          onClose={() => setSelectedResult(null)}
        >
          <p className="hint">Prompt</p>
          <p>{selectedResult.testCase.prompt}</p>

          <p className="hint">Reply</p>
          <pre className="exec-output">{selectedResult.result.reply || selectedResult.result.error || '(no reply)'}</pre>

          <p className="hint">Assertions</p>
          <ul className="event-log">
            {selectedResult.result.assertions.map((a, i) => (
              <li key={i}>
                <span className={`badge ${a.passed ? 'badge-purple' : ''}`}>{a.passed ? 'pass' : 'fail'}</span>
                <span className="event-data">
                  <code title={a.type === 'json_schema' ? a.value : undefined}>{formatAssertion(a)}</code>
                  {a.error && <div className="error">{a.error}</div>}
                </span>
              </li>
            ))}
          </ul>
        </Modal>
      )}
    </>
  )
}

export default EvaluationDetail
