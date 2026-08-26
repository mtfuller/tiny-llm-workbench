import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { deleteEvaluation, listEnvironments, listEvaluations, saveEvaluation, type Environment, type Evaluation } from '../api'
import { emptyTestCase, TestCaseFields, toPayloadTestCases, type DraftTestCase } from '../TestCaseEditor'

function Evaluations() {
  const [evaluations, setEvaluations] = useState<Evaluation[] | null>(null)
  const [environments, setEnvironments] = useState<Environment[]>([])
  const [error, setError] = useState<string | null>(null)

  const [name, setName] = useState('')
  const [environment, setEnvironment] = useState('')
  const [testCases, setTestCases] = useState<DraftTestCase[]>([emptyTestCase()])
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<string | null>(null)

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

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return

    const payloadTestCases = toPayloadTestCases(testCases)

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

  const handleDelete = async (name: string) => {
    if (!window.confirm(`Delete evaluation "${name}"? This cannot be undone.`)) return

    setDeleting(name)
    setError(null)
    try {
      await deleteEvaluation(name)
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setDeleting(null)
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

          <TestCaseFields testCases={testCases} onChange={setTestCases} />

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
                <th></th>
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
                  <td className="row-actions">
                    <button
                      type="button"
                      className="danger-button"
                      disabled={deleting === eval_.name}
                      onClick={() => handleDelete(eval_.name)}
                    >
                      {deleting === eval_.name ? 'Deleting…' : 'Delete'}
                    </button>
                  </td>
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
