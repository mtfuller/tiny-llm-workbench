import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { createKnowledgeBase, deleteKnowledgeBase, listKnowledgeBases, type KnowledgeBase } from '../api'
import { useConfirm } from '../ConfirmDialog'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'

// Knowledge is the list of KnowledgeBases — independent of any Environment
// or Docker container (see the "Knowledge binding" decision in
// CLAUDE.md): a base is just a set of records an Agent's "knowledge" node
// queries directly via deterministic keyword matching.
function Knowledge() {
  const confirm = useConfirm()
  const showToast = useToast()

  const [bases, setBases] = useState<KnowledgeBase[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)

  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const reload = () => {
    listKnowledgeBases()
      .then(setBases)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(reload, [])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return bases ?? []
    return (bases ?? []).filter((b) => b.name.toLowerCase().includes(q) || (b.description ?? '').toLowerCase().includes(q))
  }, [bases, search])

  const { page, setPage, resetPage, pageCount, pageItems } = usePagination(filtered)
  useEffect(resetPage, [search, resetPage])

  const handleCreate = async (name: string, description: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await createKnowledgeBase(name, description || undefined)
      setCreateOpen(false)
      showToast(`Created knowledge base "${name}"`)
      reload()
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (name: string) => {
    if (!(await confirm(`Delete knowledge base "${name}"? This cannot be undone.`))) return

    setDeleting(name)
    setError(null)
    try {
      await deleteKnowledgeBase(name)
      showToast(`Deleted knowledge base "${name}"`)
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
        <h2>Knowledge</h2>
      </div>
      <p className="hint">
        Records an agent's "knowledge" node can query — a deterministic keyword match against title and content, not
        embeddings or a vector store.
      </p>

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search knowledge bases…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
          <div className="list-toolbar-actions">
            <button
              type="button"
              className="icon-button"
              title="New knowledge base"
              aria-label="New knowledge base"
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

        {!error && bases === null && (
          <div className="panel-body">
            <TableSkeleton columns={4} />
          </div>
        )}

        {bases !== null && bases.length === 0 && (
          <div className="panel-body">
            <p className="hint">No knowledge bases yet. Create one above.</p>
          </div>
        )}

        {bases !== null && bases.length > 0 && filtered.length === 0 && (
          <div className="panel-body">
            <p className="hint">No knowledge bases match your search.</p>
          </div>
        )}

        {filtered.length > 0 && (
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Description</th>
                <th>Records</th>
                <th>Created</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {pageItems.map((base) => (
                <tr key={base.name}>
                  <td>
                    <Link to={`/knowledge/${encodeURIComponent(base.name)}`}>{base.name}</Link>
                  </td>
                  <td>{base.description || '—'}</td>
                  <td>{base.records.length}</td>
                  <td>{new Date(base.createdAt).toLocaleString()}</td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="icon-button"
                      title="Delete knowledge base"
                      aria-label="Delete knowledge base"
                      disabled={deleting === base.name}
                      onClick={() => handleDelete(base.name)}
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
          totalCount={bases?.length ?? 0}
          itemLabel="knowledge bases"
        />
      </div>

      {createOpen && (
        <CreateKnowledgeBaseModal creating={creating} error={createError} onCreate={handleCreate} onClose={() => setCreateOpen(false)} />
      )}
    </>
  )
}

interface CreateKnowledgeBaseModalProps {
  creating: boolean
  error: string | null
  onCreate: (name: string, description: string) => void
  onClose: () => void
}

function CreateKnowledgeBaseModal({ creating, error, onCreate, onClose }: CreateKnowledgeBaseModalProps) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    onCreate(name.trim(), description.trim())
  }

  return (
    <Modal title="New knowledge base" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </label>
        <label>
          Description (optional)
          <input type="text" value={description} onChange={(e) => setDescription(e.target.value)} />
        </label>
        <p className="hint">You'll add records on the knowledge base's own page next.</p>

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

export default Knowledge
