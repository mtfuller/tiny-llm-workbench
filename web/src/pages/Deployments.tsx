import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import {
  createDeployment,
  deleteDeployment,
  listAgents,
  listDeployments,
  listWorkspaces,
  type Agent,
  type Deployment,
  type Workspace,
} from '../api'
import IconButton from '../IconButton'
import { formatDate } from '../lib/format'
import ListPanel from '../ListPanel'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import { useResourceList } from '../useResourceList'

function Deployments() {
  const navigate = useNavigate()
  const list = useResourceList<Deployment>({
    load: listDeployments,
    getName: (d) => d.name,
    remove: (d) => deleteDeployment(d.name),
    confirmMessage: (d) => `Delete deployment "${d.name}"? This cannot be undone.`,
    deletedToast: (d) => `Deleted deployment "${d.name}"`,
  })

  const [agents, setAgents] = useState<Agent[]>([])
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  useEffect(() => {
    listAgents().then(setAgents).catch(() => setAgents([]))
    listWorkspaces().then(setWorkspaces).catch(() => setWorkspaces([]))
  }, [])

  const handleCreate = async (name: string, agentName: string, workspaceName: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await createDeployment({ name, agentName, workspaceName })
      setCreateOpen(false)
      navigate(`/deployments/${encodeURIComponent(name)}`)
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Deployments</h2>
      </div>
      <p className="hint">
        Bind an agent to a <strong>real</strong> workspace and start it to chat with the agent while it
        does actual work — its Tool/Agent nodes act directly on that directory, and the changes persist.
      </p>

      <ListPanel
        search={list.search}
        onSearch={list.setSearch}
        searchPlaceholder="Search deployments…"
        actions={<IconButton icon={<Plus size={16} />} label="New deployment" onClick={() => setCreateOpen(true)} />}
        error={list.error}
        loading={list.items === null}
        isEmpty={list.items !== null && list.items.length === 0}
        hasMatches={list.filtered.length > 0}
        emptyMessage="No deployments yet. Create one above."
        noMatchMessage="No deployments match your search."
        skeletonColumns={4}
        page={list.page}
        pageCount={list.pageCount}
        setPage={list.setPage}
        shownCount={list.filtered.length}
        totalCount={list.items?.length ?? 0}
        itemLabel="deployments"
      >
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Agent</th>
              <th>Real workspace</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.pageItems.map((d) => (
              <tr key={d.name}>
                <td>
                  <Link to={`/deployments/${encodeURIComponent(d.name)}`}>{d.name}</Link>
                </td>
                <td>{d.agentName}</td>
                <td>{d.workspaceName}</td>
                <td>{formatDate(d.createdAt)}</td>
                <td className="row-actions">
                  <IconButton
                    icon={<Trash2 size={15} />}
                    label="Delete deployment"
                    disabled={list.deleting === d.name}
                    onClick={() => list.handleDelete(d)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ListPanel>

      {createOpen && (
        <CreateDeploymentModal
          agents={agents}
          realWorkspaces={workspaces.filter((w) => w.type === 'real')}
          creating={creating}
          error={createError}
          onCreate={handleCreate}
          onClose={() => setCreateOpen(false)}
        />
      )}
    </>
  )
}

interface CreateDeploymentModalProps {
  agents: Agent[]
  realWorkspaces: Workspace[]
  creating: boolean
  error: string | null
  onCreate: (name: string, agentName: string, workspaceName: string) => void
  onClose: () => void
}

function CreateDeploymentModal({ agents, realWorkspaces, creating, error, onCreate, onClose }: CreateDeploymentModalProps) {
  const [name, setName] = useState('')
  const [agentName, setAgentName] = useState('')
  const [workspaceName, setWorkspaceName] = useState('')

  const valid = name.trim() && agentName && workspaceName

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!valid) return
    onCreate(name.trim(), agentName, workspaceName)
  }

  return (
    <Modal title="New deployment" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </label>
        <label>
          Agent
          <select value={agentName} onChange={(e) => setAgentName(e.target.value)}>
            <option value="">Select an agent…</option>
            {agents.map((a) => (
              <option key={a.name} value={a.name}>
                {a.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Real workspace
          <select value={workspaceName} onChange={(e) => setWorkspaceName(e.target.value)}>
            <option value="">Select a real workspace…</option>
            {realWorkspaces.map((w) => (
              <option key={w.name} value={w.name}>
                {w.name} — {w.hostPath}
              </option>
            ))}
          </select>
          {realWorkspaces.length === 0 && (
            <span className="field-hint">
              No real workspaces yet — create one on the <Link to="/workspaces">Workspaces</Link> page.
            </span>
          )}
        </label>

        {error && <p className="error">{error}</p>}
        <ModalActions onCancel={onClose} submitLabel="Create" busyLabel="Creating…" busy={creating} disabled={!valid} />
      </form>
    </Modal>
  )
}

export default Deployments
