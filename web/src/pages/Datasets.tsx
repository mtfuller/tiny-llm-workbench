import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { createDataset, deleteDataset, listDatasets, type DatasetSummary } from '../api'
import { useConfirm } from '../ConfirmDialog'
import Modal from '../Modal'
import { TableSkeleton } from '../Skeleton'
import { useToast } from '../Toast'

function Datasets() {
  const confirm = useConfirm()
  const showToast = useToast()
  const [datasets, setDatasets] = useState<DatasetSummary[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const reload = () => {
    listDatasets()
      .then(setDatasets)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(reload, [])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return datasets ?? []
    return (datasets ?? []).filter((d) => d.name.toLowerCase().includes(q))
  }, [datasets, search])

  const handleCreate = async (name: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await createDataset(name)
      setCreateOpen(false)
      reload()
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (name: string) => {
    if (!(await confirm(`Delete dataset "${name}"? This cannot be undone.`))) return

    setDeleting(name)
    setError(null)
    try {
      await deleteDataset(name)
      showToast(`Deleted dataset "${name}"`)
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
        <h2>Datasets</h2>
      </div>
      <p className="hint">Input/output training pairs used to fine-tune a model.</p>

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search datasets…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
          <div className="list-toolbar-actions">
            <button type="button" className="icon-button" title="New dataset" aria-label="New dataset" onClick={() => setCreateOpen(true)}>
              <Plus size={16} />
            </button>
          </div>
        </div>

        {error && (
          <div className="panel-body">
            <p className="error">{error}</p>
          </div>
        )}

        {!error && datasets === null && (
          <div className="panel-body">
            <TableSkeleton columns={3} />
          </div>
        )}

        {datasets !== null && datasets.length === 0 && (
          <div className="panel-body">
            <p className="hint">No datasets yet. Create one above to get started.</p>
          </div>
        )}

        {datasets !== null && datasets.length > 0 && filtered.length === 0 && (
          <div className="panel-body">
            <p className="hint">No datasets match your search.</p>
          </div>
        )}

        {filtered.length > 0 && (
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Pairs</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((dataset) => (
                <tr key={dataset.name}>
                  <td>
                    <Link to={`/datasets/${encodeURIComponent(dataset.name)}`}>{dataset.name}</Link>
                  </td>
                  <td>{dataset.pairCount}</td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="icon-button"
                      title="Delete dataset"
                      aria-label="Delete dataset"
                      disabled={deleting === dataset.name}
                      onClick={() => handleDelete(dataset.name)}
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
        <CreateDatasetModal
          creating={creating}
          error={createError}
          onCreate={handleCreate}
          onClose={() => setCreateOpen(false)}
        />
      )}
    </>
  )
}

interface CreateDatasetModalProps {
  creating: boolean
  error: string | null
  onCreate: (name: string) => void
  onClose: () => void
}

function CreateDatasetModal({ creating, error, onCreate, onClose }: CreateDatasetModalProps) {
  const [name, setName] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    onCreate(name.trim())
  }

  return (
    <Modal title="New dataset" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </label>
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

export default Datasets
