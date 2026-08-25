import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  listEnvironments,
  listEvaluations,
  saveEvaluation,
  type AssertionType,
  type Environment,
  type Evaluation,
  type TestCase,
} from '../api'

interface DraftAssertion {
  type: AssertionType
  value: string
}

interface DraftTestCase {
  prompt: string
  assertions: DraftAssertion[]
}

function emptyTestCase(): DraftTestCase {
  return { prompt: '', assertions: [{ type: 'contains', value: '' }] }
}

function Evaluations() {
  const [evaluations, setEvaluations] = useState<Evaluation[] | null>(null)
  const [environments, setEnvironments] = useState<Environment[]>([])
  const [error, setError] = useState<string | null>(null)

  const [name, setName] = useState('')
  const [environment, setEnvironment] = useState('')
  const [testCases, setTestCases] = useState<DraftTestCase[]>([emptyTestCase()])
  const [creating, setCreating] = useState(false)

  const reload = () => {
    listEvaluations()
      .then(setEvaluations)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    reload()
    listEnvironments()
      .then(setEnvironments)
      .catch(() => setEnvironments([]))
  }, [])

  const updateTestCase = (i: number, patch: Partial<DraftTestCase>) => {
    setTestCases((prev) => prev.map((tc, idx) => (idx === i ? { ...tc, ...patch } : tc)))
  }

  const updateAssertion = (tcIndex: number, aIndex: number, patch: Partial<DraftAssertion>) => {
    setTestCases((prev) =>
      prev.map((tc, idx) =>
        idx === tcIndex
          ? { ...tc, assertions: tc.assertions.map((a, ai) => (ai === aIndex ? { ...a, ...patch } : a)) }
          : tc,
      ),
    )
  }

  const addAssertion = (tcIndex: number) => {
    setTestCases((prev) =>
      prev.map((tc, idx) => (idx === tcIndex ? { ...tc, assertions: [...tc.assertions, { type: 'contains', value: '' }] } : tc)),
    )
  }

  const removeAssertion = (tcIndex: number, aIndex: number) => {
    setTestCases((prev) =>
      prev.map((tc, idx) => (idx === tcIndex ? { ...tc, assertions: tc.assertions.filter((_, ai) => ai !== aIndex) } : tc)),
    )
  }

  const addTestCase = () => setTestCases((prev) => [...prev, emptyTestCase()])
  const removeTestCase = (i: number) => setTestCases((prev) => prev.filter((_, idx) => idx !== i))

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return

    const payloadTestCases: TestCase[] = testCases
      .filter((tc) => tc.prompt.trim())
      .map((tc, i) => ({
        id: `tc-${i}`,
        prompt: tc.prompt.trim(),
        assertions: tc.assertions.filter((a) => a.value.trim()).map((a) => ({ type: a.type, value: a.value.trim() })),
      }))

    if (payloadTestCases.length === 0) {
      setError('At least one test case with a prompt is required.')
      return
    }

    setCreating(true)
    setError(null)
    try {
      await saveEvaluation({ name: name.trim(), environment: environment || undefined, testCases: payloadTestCases })
      setName('')
      setEnvironment('')
      setTestCases([emptyTestCase()])
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Evaluations</h2>
      </div>
      <p className="hint">
        Define test cases (a prompt plus assertions on the reply) and run them against a set of agents
        to compare how they perform.
      </p>

      {error && <p className="error">{error}</p>}

      <section className="panel">
        <h3>New evaluation</h3>
        <form className="stacked-form" onSubmit={handleCreate}>
          <label>
            Name
            <input type="text" value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label>
            Environment (optional)
            <select value={environment} onChange={(e) => setEnvironment(e.target.value)}>
              <option value="">None</option>
              {environments.map((env) => (
                <option key={env.name} value={env.name}>
                  {env.name}
                </option>
              ))}
            </select>
          </label>

          {testCases.map((tc, tcIndex) => (
            <div key={tcIndex} className="test-case-editor">
              <div className="page-header">
                <strong>Test case {tcIndex + 1}</strong>
                {testCases.length > 1 && (
                  <button type="button" className="danger-button" onClick={() => removeTestCase(tcIndex)}>
                    Remove
                  </button>
                )}
              </div>
              <label>
                Prompt
                <input
                  type="text"
                  placeholder="say hello"
                  value={tc.prompt}
                  onChange={(e) => updateTestCase(tcIndex, { prompt: e.target.value })}
                />
              </label>
              {tc.assertions.map((a, aIndex) => (
                <div className="assertion-row" key={aIndex}>
                  <select
                    value={a.type}
                    onChange={(e) => updateAssertion(tcIndex, aIndex, { type: e.target.value as AssertionType })}
                  >
                    <option value="contains">contains</option>
                    <option value="not_contains">not contains</option>
                    <option value="regex">matches regex</option>
                  </select>
                  <input
                    type="text"
                    placeholder="value"
                    value={a.value}
                    onChange={(e) => updateAssertion(tcIndex, aIndex, { value: e.target.value })}
                  />
                  {tc.assertions.length > 1 && (
                    <button type="button" className="danger-button" onClick={() => removeAssertion(tcIndex, aIndex)}>
                      ×
                    </button>
                  )}
                </div>
              ))}
              <button type="button" onClick={() => addAssertion(tcIndex)}>
                + Assertion
              </button>
            </div>
          ))}
          <button type="button" onClick={addTestCase}>
            + Test case
          </button>

          <button type="submit" disabled={creating || !name.trim()}>
            {creating ? 'Creating…' : 'Create evaluation'}
          </button>
        </form>
      </section>

      {!error && evaluations === null && <p className="hint">Loading…</p>}

      {evaluations !== null && evaluations.length === 0 && (
        <p className="empty-state">No evaluations yet. Create one above.</p>
      )}

      {evaluations !== null && evaluations.length > 0 && (
        <div className="panel panel-flush">
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Environment</th>
                <th>Test cases</th>
              </tr>
            </thead>
            <tbody>
              {evaluations.map((eval_) => (
                <tr key={eval_.name}>
                  <td>
                    <Link to={`/evaluations/${encodeURIComponent(eval_.name)}`}>{eval_.name}</Link>
                  </td>
                  <td>{eval_.environment || '—'}</td>
                  <td>{eval_.testCases.length}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}

export default Evaluations
