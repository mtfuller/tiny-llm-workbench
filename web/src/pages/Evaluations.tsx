import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { deleteEvaluation, listEnvironments, listEvaluations, saveEvaluation, type Environment, type Evaluation } from '../api'
import { useConfirm } from '../ConfirmDialog'
import Modal from '../Modal'
import { TableSkeleton } from '../Skeleton'
import { emptyTestCase, TestCaseFields, toPayloadTestCases, type DraftTestCase } from '../TestCaseEditor'
import { useToast } from '../Toast'

function Evaluations() {
  const confirm = useConfirm()
  const showToast = useToast()
  const [evaluations, setEvaluations] = useState<Evaluation[] | null>(null)
  const [environments, setEnvironments] = useState<Environment[]>([])
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)

  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

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

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return evaluations ?? []
    return (evaluations ?? []).filter((e) => e.name.toLowerCase().includes(q))
  }, [evaluations, search])

  const handleCreate = async (name: string, environment: string, testCases: DraftTestCase[]) => {
    const payloadTestCases = toPayloadTestCases(testCases)

    if (payloadTestCases.length === 0) {
      setCreateError('At least one test case with a prompt is required.')
      return
    }

    setCreating(true)
    setCreateError(null)
    try {
      await saveEvaluation({ name, environment: environment || undefined, testCases: payloadTestCases })
      setCreateOpen(false)
      reload()
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (name: string) => {
    if (!(await confirm(`Delete evaluation "${name}"? This cannot be undone.`))) return

    setDeleting(name)
    setError(null)
    try {
      await deleteEvaluation(name)
      showToast(`Deleted evaluation "${name}"`)
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

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search evaluations…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
          <div className="list-toolbar-actions">
            <button
              type="button"
              className="icon-button"
              title="New evaluation"
              aria-label="New evaluation"
              onClick={() => setCreateOpen(true)}
            >
              <Plus size={16} />
            </button>
          </div>
        </div>

        {error && (
          <div className="panel-body">
            <p className="error">{error}</p>
          </div>
        )}

        {!error && evaluations === null && (
          <div className="panel-body">
            <TableSkeleton columns={4} />
          </div>
        )}

        {evaluations !== null && evaluations.length === 0 && (
          <div className="panel-body">
            <p className="hint">No evaluations yet. Create one above.</p>
          </div>
        )}

        {evaluations !== null && evaluations.length > 0 && filtered.length === 0 && (
          <div className="panel-body">
            <p className="hint">No evaluations match your search.</p>
          </div>
        )}

        {filtered.length > 0 && (
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
              {filtered.map((eval_) => (
                <tr key={eval_.name}>
                  <td>
                    <Link to={`/evaluations/${encodeURIComponent(eval_.name)}`}>{eval_.name}</Link>
                  </td>
                  <td>{eval_.environment || '—'}</td>
                  <td>{eval_.testCases.length}</td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="icon-button"
                      title="Delete evaluation"
                      aria-label="Delete evaluation"
                      disabled={deleting === eval_.name}
                      onClick={() => handleDelete(eval_.name)}
                    >
                      <Trash2 size={15} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {createOpen && (
        <CreateEvaluationModal
          environments={environments}
          creating={creating}
          error={createError}
          onCreate={handleCreate}
          onClose={() => setCreateOpen(false)}
        />
      )}
    </>
  )
}

interface CreateEvaluationModalProps {
  environments: Environment[]
  creating: boolean
  error: string | null
  onCreate: (name: string, environment: string, testCases: DraftTestCase[]) => void
  onClose: () => void
}

function CreateEvaluationModal({ environments, creating, error, onCreate, onClose }: CreateEvaluationModalProps) {
  const [name, setName] = useState('')
  const [environment, setEnvironment] = useState('')
  const [testCases, setTestCases] = useState<DraftTestCase[]>([emptyTestCase()])

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    onCreate(name.trim(), environment, testCases)
  }

  return (
    <Modal title="New evaluation" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
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

        <div className="test-case-list">
          <TestCaseFields testCases={testCases} onChange={setTestCases} />
        </div>

        {error && <p className="error">{error}</p>}
        <div className="row-actions confirm-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={creating || !name.trim()}>
            {creating ? 'Creating…' : 'Create'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

export default Evaluations
