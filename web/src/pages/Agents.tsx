import { Plus, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { deleteAgent, listAgents, saveAgent, type Agent } from '../api'
import IconButton from '../IconButton'
import { formatDate } from '../lib/format'
import ListPanel from '../ListPanel'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import { useResourceList } from '../useResourceList'

function Agents() {
  const navigate = useNavigate()
  const list = useResourceList<Agent>({
    load: listAgents,
    getName: (a) => a.name,
    remove: (a) => deleteAgent(a.name),
    confirmMessage: (a) => `Delete agent "${a.name}"? This cannot be undone.`,
    deletedToast: (a) => `Deleted agent "${a.name}"`,
  })

  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const handleCreate = async (name: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      // The canvas flows top-to-bottom, so the Input node starts near the top.
      await saveAgent(
        name,
        {
          nodes: [{ id: 'input-1', type: 'input', position: { x: 240, y: 60 }, data: { name: 'Input' } }],
          edges: [],
        },
        { tools: [], knowledgeBases: [] },
      )
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

      <ListPanel
        search={list.search}
        onSearch={list.setSearch}
        searchPlaceholder="Search agents…"
        actions={<IconButton icon={<Plus size={16} />} label="New agent" onClick={() => setCreateOpen(true)} />}
        error={list.error}
        loading={list.items === null}
        isEmpty={list.items !== null && list.items.length === 0}
        hasMatches={list.filtered.length > 0}
        emptyMessage="No agents yet. Create one above to open the canvas."
        noMatchMessage="No agents match your search."
        skeletonColumns={5}
        page={list.page}
        pageCount={list.pageCount}
        setPage={list.setPage}
        shownCount={list.filtered.length}
        totalCount={list.items?.length ?? 0}
        itemLabel="agents"
      >
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Workspace</th>
              <th>Nodes</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.pageItems.map((agent) => (
              <tr key={agent.name}>
                <td>
                  <Link to={`/agents/${encodeURIComponent(agent.name)}`}>{agent.name}</Link>
                </td>
                <td>{agent.workspace || '—'}</td>
                <td>{agent.graph.nodes.length}</td>
                <td>{formatDate(agent.createdAt)}</td>
                <td className="row-actions">
                  <IconButton
                    icon={<Trash2 size={15} />}
                    label="Delete agent"
                    disabled={list.deleting === agent.name}
                    onClick={() => list.handleDelete(agent)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ListPanel>

      {createOpen && (
        <CreateAgentModal creating={creating} error={createError} onCreate={handleCreate} onClose={() => setCreateOpen(false)} />
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
        <ModalActions onCancel={onClose} submitLabel="Create" busyLabel="Creating…" busy={creating} disabled={!name.trim()} />
      </form>
    </Modal>
  )
}

export default Agents
