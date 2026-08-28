import { Plus, Square, Trash2 } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import {
  createWorkspace,
  deleteWorkspace,
  listInstances,
  listWorkspaces,
  stopInstance,
  type Instance,
  type Workspace,
  type WorkspaceType,
} from '../api'
import { Badge } from '../Badge'
import DirectoryPicker from '../DirectoryPicker'
import IconButton from '../IconButton'
import ListPanel from '../ListPanel'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { useResourceList } from '../useResourceList'
import { usePagination } from '../usePagination'

function Workspaces() {
  const list = useResourceList<Workspace>({
    load: listWorkspaces,
    getName: (w) => w.name,
    remove: (w) => deleteWorkspace(w.name),
    confirmMessage: (w) =>
      w.type === 'test'
        ? `Delete test workspace "${w.name}"? Its files under ~/.tlw will be removed.`
        : `Delete workspace "${w.name}"? The directory on your machine is not touched — only this pointer to it.`,
    deletedToast: (w) => `Deleted workspace "${w.name}"`,
  })

  const [instances, setInstances] = useState<Instance[] | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [stopping, setStopping] = useState<string | null>(null)

  const reloadInstances = () => {
    listInstances()
      .then(setInstances)
      .catch((err: Error) => list.setError(err.message))
  }

  useEffect(() => {
    reloadInstances()
  }, [])
  useEffect(() => {
    const interval = setInterval(reloadInstances, 4000)
    return () => clearInterval(interval)
  }, [])

  const instancePages = usePagination(instances ?? [])

  const handleCreate = async (name: string, type: WorkspaceType, hostPath?: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await createWorkspace({ name, type, hostPath })
      setCreateOpen(false)
      list.reload()
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const handleStop = async (id: string) => {
    setStopping(id)
    list.setError(null)
    try {
      await stopInstance(id)
      reloadInstances()
    } catch (err) {
      list.setError((err as Error).message)
    } finally {
      setStopping(null)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Workspaces</h2>
      </div>
      <p className="hint">
        A workspace is just a directory an agent works in. A <strong>test</strong> workspace lives under
        ~/.tlw and is <em>copied</em> into a fresh sandbox per run (changes don't persist — for
        experimenting and evaluations). A <strong>real</strong> workspace points at a folder on your
        machine and its changes persist — use those with Deployments for actual work.
      </p>

      {list.error && <p className="error">{list.error}</p>}

      <ListPanel
        search={list.search}
        onSearch={list.setSearch}
        searchPlaceholder="Search workspaces…"
        actions={<IconButton icon={<Plus size={16} />} label="New workspace" onClick={() => setCreateOpen(true)} />}
        loading={list.items === null}
        isEmpty={list.items !== null && list.items.length === 0}
        hasMatches={list.filtered.length > 0}
        emptyMessage="No workspaces yet. Create one above."
        noMatchMessage="No workspaces match your search."
        skeletonColumns={4}
        page={list.page}
        pageCount={list.pageCount}
        setPage={list.setPage}
        shownCount={list.filtered.length}
        totalCount={list.items?.length ?? 0}
        itemLabel="workspaces"
      >
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Location</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.pageItems.map((ws) => (
              <tr key={ws.name}>
                <td>
                  <Link to={`/workspaces/${encodeURIComponent(ws.name)}`}>{ws.name}</Link>
                </td>
                <td>
                  <Badge>{ws.type}</Badge>
                </td>
                <td>
                  <code>{ws.hostPath}</code>
                </td>
                <td className="row-actions">
                  <IconButton
                    icon={<Trash2 size={15} />}
                    label="Delete workspace"
                    disabled={list.deleting === ws.name}
                    onClick={() => list.handleDelete(ws)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ListPanel>

      <div className="page-header">
        <h3>Running sandboxes</h3>
      </div>

      {instances === null && <TableSkeleton columns={3} />}

      {instances !== null && instances.length === 0 && (
        <p className="empty-state">No sandboxes running. They start when an agent run, debug session, deployment, or the Tools playground needs one.</p>
      )}

      {instances !== null && instances.length > 0 && (
        <div className="panel panel-flush">
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Workspace</th>
                <th>State</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {instancePages.pageItems.map((instance) => (
                <tr key={instance.id}>
                  <td>{instance.name}</td>
                  <td>{instance.workspaceName || '—'}</td>
                  <td>
                    <span className={`status ${instance.state === 'running' ? 'status-open' : 'status-closed'}`}>
                      {instance.state}
                    </span>
                  </td>
                  <td className="row-actions">
                    <IconButton
                      icon={<Square size={15} />}
                      label="Stop"
                      disabled={stopping === instance.id}
                      onClick={() => handleStop(instance.id)}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pagination
            page={instancePages.page}
            pageCount={instancePages.pageCount}
            onChange={instancePages.setPage}
            shownCount={instances.length}
            totalCount={instances.length}
            itemLabel="sandboxes"
          />
        </div>
      )}

      {createOpen && (
        <CreateWorkspaceModal creating={creating} error={createError} onCreate={handleCreate} onClose={() => setCreateOpen(false)} />
      )}
    </>
  )
}

interface CreateWorkspaceModalProps {
  creating: boolean
  error: string | null
  onCreate: (name: string, type: WorkspaceType, hostPath?: string) => void
  onClose: () => void
}

function CreateWorkspaceModal({ creating, error, onCreate, onClose }: CreateWorkspaceModalProps) {
  const [name, setName] = useState('')
  const [type, setType] = useState<WorkspaceType>('test')
  const [hostPath, setHostPath] = useState('')
  const [pickerOpen, setPickerOpen] = useState(false)

  const valid = name.trim() && (type === 'test' || hostPath.trim())

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!valid) return
    onCreate(name.trim(), type, type === 'real' ? hostPath.trim() : undefined)
  }

  return (
    <Modal title="New workspace" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </label>

        <fieldset className="radio-group">
          <legend>Type</legend>
          <label className="radio-label">
            <input type="radio" name="ws-type" checked={type === 'test'} onChange={() => setType('test')} />
            <span>
              <strong>Test</strong> — a folder is created under ~/.tlw. Edit it in your editor to set up a
              starting scenario. Copied into a fresh sandbox per run; changes never flow back.
            </span>
          </label>
          <label className="radio-label">
            <input type="radio" name="ws-type" checked={type === 'real'} onChange={() => setType('real')} />
            <span>
              <strong>Real</strong> — point at a directory on your machine, bind-mounted so an agent's
              changes persist. Used by Deployments.
            </span>
          </label>
        </fieldset>

        {type === 'real' && (
          <label>
            Directory
            <div className="input-with-button">
              <input
                type="text"
                placeholder="/Users/you/my-project"
                value={hostPath}
                onChange={(e) => setHostPath(e.target.value)}
              />
              <button type="button" className="button-secondary" onClick={() => setPickerOpen(true)}>
                Browse…
              </button>
            </div>
          </label>
        )}

        {error && <p className="error">{error}</p>}
        <ModalActions onCancel={onClose} submitLabel="Create" busyLabel="Creating…" busy={creating} disabled={!valid} />
      </form>

      {pickerOpen && (
        <DirectoryPicker
          initialPath={hostPath.trim() || undefined}
          onSelect={(path) => {
            setHostPath(path)
            setPickerOpen(false)
          }}
          onClose={() => setPickerOpen(false)}
        />
      )}
    </Modal>
  )
}

export default Workspaces
