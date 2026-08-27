import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { deleteBenchmark, listBenchmarks, saveBenchmark, type Benchmark } from '../api'
import { useConfirm } from '../ConfirmDialog'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'

function Benchmarks() {
  const confirm = useConfirm()
  const showToast = useToast()
  const navigate = useNavigate()
  const [benchmarks, setBenchmarks] = useState<Benchmark[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)

  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const reload = () => {
    listBenchmarks()
      .then(setBenchmarks)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(reload, [])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return benchmarks ?? []
    return (benchmarks ?? []).filter((b) => b.name.toLowerCase().includes(q))
  }, [benchmarks, search])

  const { page, setPage, resetPage, pageCount, pageItems } = usePagination(filtered)

  useEffect(resetPage, [search, resetPage])

  const handleCreate = async (name: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await saveBenchmark({ name, testCases: [] })
      setCreateOpen(false)
      // A brand-new benchmark has no test cases yet — send the user
      // straight to its detail page to add some, rather than leaving them
      // on the list looking at an empty row.
      navigate(`/benchmarks/${encodeURIComponent(name)}`)
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (name: string) => {
    if (!(await confirm(`Delete benchmark "${name}"? This cannot be undone.`))) return

    setDeleting(name)
    setError(null)
    try {
      await deleteBenchmark(name)
      showToast(`Deleted benchmark "${name}"`)
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
        <h2>Benchmarks</h2>
      </div>
      <p className="hint">
        Define test cases (a prompt plus assertions on the reply) and run them against a set of models
        to compare how they perform.
      </p>

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search benchmarks…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
          <div className="list-toolbar-actions">
            <button
              type="button"
              className="icon-button"
              title="New benchmark"
              aria-label="New benchmark"
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

        {!error && benchmarks === null && (
          <div className="panel-body">
            <TableSkeleton columns={4} />
          </div>
        )}

        {benchmarks !== null && benchmarks.length === 0 && (
          <div className="panel-body">
            <p className="hint">No benchmarks yet. Create one above.</p>
          </div>
        )}

        {benchmarks !== null && benchmarks.length > 0 && filtered.length === 0 && (
          <div className="panel-body">
            <p className="hint">No benchmarks match your search.</p>
          </div>
        )}

        {filtered.length > 0 && (
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Version</th>
                <th>Test cases</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {pageItems.map((b) => (
                <tr key={b.name}>
                  <td>
                    <Link to={`/benchmarks/${encodeURIComponent(b.name)}`}>{b.name}</Link>
                  </td>
                  <td>v{b.version}</td>
                  <td>{b.testCases.length}</td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="icon-button"
                      title="Delete benchmark"
                      aria-label="Delete benchmark"
                      disabled={deleting === b.name}
                      onClick={() => handleDelete(b.name)}
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
          totalCount={benchmarks?.length ?? 0}
          itemLabel="benchmarks"
        />
      </div>

      {createOpen && (
        <CreateBenchmarkModal creating={creating} error={createError} onCreate={handleCreate} onClose={() => setCreateOpen(false)} />
      )}
    </>
  )
}

interface CreateBenchmarkModalProps {
  creating: boolean
  error: string | null
  onCreate: (name: string) => void
  onClose: () => void
}

function CreateBenchmarkModal({ creating, error, onCreate, onClose }: CreateBenchmarkModalProps) {
  const [name, setName] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    onCreate(name.trim())
  }

  return (
    <Modal title="New benchmark" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </label>
        <p className="hint">You'll add test cases on the benchmark's own page next.</p>

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

export default Benchmarks
