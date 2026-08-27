import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { deleteEvaluation, listEnvironments, listEvaluations, saveEvaluation, type Environment, type Evaluation } from '../api'
import { useConfirm } from '../ConfirmDialog'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'

function Evaluations() {
  const confirm = useConfirm()
  const showToast = useToast()
  const navigate = useNavigate()
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

  const { page, setPage, resetPage, pageCount, pageItems } = usePagination(filtered)

  useEffect(resetPage, [search, resetPage])

  const handleCreate = async (name: string, environment: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await saveEvaluation({ name, environment: environment || undefined })
      setCreateOpen(false)
      // A brand-new evaluation has no test cases yet — send the user
      // straight to its detail page to add some, rather than leaving them
      // on the list looking at an empty row.
      navigate(`/evaluations/${encodeURIComponent(name)}`)
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
        Define test cases — a prompt, optional setup commands to prepare a scenario, and assertions on
        the reply and the environment's resulting state — and run them against a set of agents to see how
        well they actually complete real software-dev, knowledge-work, or office-work tasks.
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
            <TableSkeleton columns={5} />
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
                <th>Version</th>
                <th>Test cases</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {pageItems.map((eval_) => (
                <tr key={eval_.name}>
                  <td>
                    <Link to={`/evaluations/${encodeURIComponent(eval_.name)}`}>{eval_.name}</Link>
                  </td>
                  <td>{eval_.environment || '—'}</td>
                  <td>v{eval_.version}</td>
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

        <Pagination
          page={page}
          pageCount={pageCount}
          onChange={setPage}
          shownCount={filtered.length}
          totalCount={evaluations?.length ?? 0}
          itemLabel="evaluations"
        />
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
  onCreate: (name: string, environment: string) => void
  onClose: () => void
}

function CreateEvaluationModal({ environments, creating, error, onCreate, onClose }: CreateEvaluationModalProps) {
  const [name, setName] = useState('')
  const [environment, setEnvironment] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    onCreate(name.trim(), environment)
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
          <span className="field-hint">
            Needed only if a test case's setup/verify commands should run against a real environment.
          </span>
        </label>
        <p className="hint">You'll add test cases on the evaluation's own page next.</p>

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
