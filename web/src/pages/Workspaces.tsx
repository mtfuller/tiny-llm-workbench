import { Play, Plus, Square, Trash2 } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import {
  createEnvironment,
  deleteEnvironment,
  launchEnvironment,
  listEnvironments,
  listInstances,
  stopInstance,
  type Environment,
  type Instance,
} from '../api'
import { Badge } from '../Badge'
import IconButton from '../IconButton'
import ListPanel from '../ListPanel'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { useResourceList } from '../useResourceList'
import { usePagination } from '../usePagination'

function Environments() {
  const list = useResourceList<Environment>({
    load: listEnvironments,
    getName: (e) => e.name,
    remove: (e) => deleteEnvironment(e.name),
    confirmMessage: (e) => `Delete environment "${e.name}"? This cannot be undone.`,
    deletedToast: (e) => `Deleted environment "${e.name}"`,
  })

  const [instances, setInstances] = useState<Instance[] | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [launching, setLaunching] = useState<string | null>(null)
  const [stopping, setStopping] = useState<string | null>(null)

  const reloadInstances = () => {
    listInstances()
      .then(setInstances)
      .catch((err: Error) => list.setError(err.message))
  }

  useEffect(() => {
    reloadInstances()
  }, [])
  // Poll instances — starting/stopping from an environment's own workspace
  // page doesn't otherwise notify this page.
  useEffect(() => {
    const interval = setInterval(reloadInstances, 4000)
    return () => clearInterval(interval)
  }, [])

  const instancePages = usePagination(instances ?? [])

  const handleCreate = async (name: string, image: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await createEnvironment({ name, image, mounts: [] })
      setCreateOpen(false)
      list.reload()
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const handleLaunch = async (name: string) => {
    setLaunching(name)
    list.setError(null)
    try {
      await launchEnvironment(name)
      reloadInstances()
    } catch (err) {
      list.setError((err as Error).message)
    } finally {
      setLaunching(null)
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
        <h2>Environments</h2>
      </div>
      <p className="hint">
        Sandboxed Docker containers agents can act inside. Build one out — image, mounts, tools — in its own
        workspace, then launch it and try a tool.
      </p>

      {list.error && <p className="error">{list.error}</p>}

      <div className="page-header">
        <h3>Definitions</h3>
      </div>

      <ListPanel
        search={list.search}
        onSearch={list.setSearch}
        searchPlaceholder="Search environments…"
        actions={
          <IconButton icon={<Plus size={16} />} label="Create a custom environment" onClick={() => setCreateOpen(true)} />
        }
        loading={list.items === null}
        isEmpty={list.items !== null && list.items.length === 0}
        hasMatches={list.filtered.length > 0}
        emptyMessage="No environment definitions found."
        noMatchMessage="No environments match your search."
        skeletonColumns={5}
        page={list.page}
        pageCount={list.pageCount}
        setPage={list.setPage}
        shownCount={list.filtered.length}
        totalCount={list.items?.length ?? 0}
        itemLabel="environments"
      >
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Image</th>
              <th>Tools</th>
              <th>Mounts</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.pageItems.map((env) => (
              <tr key={env.name}>
                <td>
                  <Link to={`/environments/${encodeURIComponent(env.name)}`}>{env.name}</Link>{' '}
                  {env.prebuilt && <Badge>prebuilt</Badge>}
                </td>
                <td>
                  <code>{env.image}</code>
                </td>
                <td>{env.tools.length}</td>
                <td>{env.mounts.length}</td>
                <td className="row-actions">
                  <IconButton
                    icon={<Play size={15} />}
                    label="Launch"
                    disabled={launching === env.name}
                    onClick={() => handleLaunch(env.name)}
                  />
                  <IconButton
                    icon={<Trash2 size={15} />}
                    label="Delete environment"
                    disabled={list.deleting === env.name}
                    onClick={() => list.handleDelete(env)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ListPanel>

      <div className="page-header">
        <h3>Running instances</h3>
      </div>

      {instances === null && <TableSkeleton columns={4} />}

      {instances !== null && instances.length === 0 && (
        <p className="empty-state">No instances running. Launch a definition above.</p>
      )}

      {instances !== null && instances.length > 0 && (
        <div className="panel panel-flush">
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Environment</th>
                <th>State</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {instancePages.pageItems.map((instance) => (
                <tr key={instance.id}>
                  <td>{instance.name}</td>
                  <td>
                    <Link to={`/environments/${encodeURIComponent(instance.environmentName)}`}>{instance.environmentName}</Link>
                  </td>
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
            itemLabel="instances"
          />
        </div>
      )}

      {createOpen && (
        <CreateEnvironmentModal creating={creating} error={createError} onCreate={handleCreate} onClose={() => setCreateOpen(false)} />
      )}
    </>
  )
}

interface CreateEnvironmentModalProps {
  creating: boolean
  error: string | null
  onCreate: (name: string, image: string) => void
  onClose: () => void
}

function CreateEnvironmentModal({ creating, error, onCreate, onClose }: CreateEnvironmentModalProps) {
  const [name, setName] = useState('')
  const [image, setImage] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !image.trim()) return
    onCreate(name.trim(), image.trim())
  }

  return (
    <Modal title="Create a custom environment" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </label>
        <label>
          Docker image
          <input type="text" placeholder="alpine:3.20" value={image} onChange={(e) => setImage(e.target.value)} />
        </label>
        <p className="hint">Add mounts and tools afterward from the environment's own workspace page.</p>

        {error && <p className="error">{error}</p>}
        <ModalActions onCancel={onClose} submitLabel="Create" busyLabel="Creating…" busy={creating} disabled={!name.trim() || !image.trim()} />
      </form>
    </Modal>
  )
}

export default Environments
