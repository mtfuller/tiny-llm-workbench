import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { deleteAgent, listAgents, saveAgent, type Agent } from '../api'
import { useConfirm } from '../ConfirmDialog'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'

function Agents() {
  const navigate = useNavigate()
  const confirm = useConfirm()
  const showToast = useToast()
  const [agents, setAgents] = useState<Agent[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const reload = () => {
    listAgents()
      .then(setAgents)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(reload, [])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return agents ?? []
    return (agents ?? []).filter((a) => a.name.toLowerCase().includes(q))
  }, [agents, search])

  const { page, setPage, resetPage, pageCount, pageItems } = usePagination(filtered)

  useEffect(resetPage, [search, resetPage])

  const handleDelete = async (name: string) => {
    if (!(await confirm(`Delete agent "${name}"? This cannot be undone.`))) return

    setDeleting(name)
    setError(null)
    try {
      await deleteAgent(name)
      showToast(`Deleted agent "${name}"`)
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setDeleting(null)
    }
  }

  const handleCreate = async (name: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      const startX = 60
      await saveAgent(name, {
        nodes: [
          { id: 'input-1', type: 'input', position: { x: startX, y: 120 }, data: { label: 'Input' } },
          { id: 'output-1', type: 'output', position: { x: startX + 500, y: 120 }, data: { label: 'Output' } },
        ],
        edges: [],
      })
      navigate(`/agents/${encodeURIComponent(name)}`)
    } catch (err) {
      setCreateError((err as Error).message)
      setCreating(false)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Agents</h2>
      </div>
      <p className="hint">Design agent workflows on a canvas, then chat with them to try them out.</p>

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search agents…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
          <div className="list-toolbar-actions">
            <button type="button" className="icon-button" title="New agent" aria-label="New agent" onClick={() => setCreateOpen(true)}>
              <Plus size={16} />
            </button>
          </div>
        </div>

        {error && (
          <div className="panel-body">
            <p className="error">{error}</p>
          </div>
        )}

        {!error && agents === null && (
          <div className="panel-body">
            <TableSkeleton columns={5} />
          </div>
        )}

        {agents !== null && agents.length === 0 && (
          <div className="panel-body">
            <p className="hint">No agents yet. Create one above to open the canvas.</p>
          </div>
        )}

        {agents !== null && agents.length > 0 && filtered.length === 0 && (
          <div className="panel-body">
            <p className="hint">No agents match your search.</p>
          </div>
        )}

        {filtered.length > 0 && (
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Environment</th>
                <th>Nodes</th>
                <th>Created</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {pageItems.map((agent) => (
                <tr key={agent.name}>
                  <td>
                    <Link to={`/agents/${encodeURIComponent(agent.name)}`}>{agent.name}</Link>
                  </td>
                  <td>{agent.environment || '—'}</td>
                  <td>{agent.graph.nodes.length}</td>
                  <td>{new Date(agent.createdAt).toLocaleDateString()}</td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="icon-button"
                      title="Delete agent"
                      aria-label="Delete agent"
                      disabled={deleting === agent.name}
                      onClick={() => handleDelete(agent.name)}
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
          totalCount={agents?.length ?? 0}
          itemLabel="agents"
        />
      </div>

      {createOpen && (
        <CreateAgentModal
          creating={creating}
          error={createError}
          onCreate={handleCreate}
          onClose={() => setCreateOpen(false)}
        />
      )}
    </>
  )
}

interface CreateAgentModalProps {
  creating: boolean
  error: string | null
  onCreate: (name: string) => void
  onClose: () => void
}

function CreateAgentModal({ creating, error, onCreate, onClose }: CreateAgentModalProps) {
  const [name, setName] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    onCreate(name.trim())
  }

  return (
    <Modal title="New agent" onClose={onClose}>
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

export default Agents
