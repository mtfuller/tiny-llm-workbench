import { Play, Plus, Square, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { createEnvironment, deleteEnvironment, launchEnvironment, listEnvironments, listInstances, stopInstance, type Environment, type Instance } from '../api'
import { useConfirm } from '../ConfirmDialog'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'

function Environments() {
  const confirm = useConfirm()
  const showToast = useToast()

  const [environments, setEnvironments] = useState<Environment[] | null>(null)
  const [instances, setInstances] = useState<Instance[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const [launching, setLaunching] = useState<string | null>(null)
  const [stopping, setStopping] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<string | null>(null)

  const reloadEnvironments = () => {
    listEnvironments()
      .then(setEnvironments)
      .catch((err: Error) => setError(err.message))
  }

  const reloadInstances = () => {
    listInstances()
      .then(setInstances)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    reloadEnvironments()
    reloadInstances()
  }, [])

  // Poll instances, since starting/stopping instances from an environment's
  // own workspace page doesn't otherwise notify this page.
  useEffect(() => {
    const interval = setInterval(reloadInstances, 4000)
    return () => clearInterval(interval)
  }, [])

  const filteredEnvironments = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return environments ?? []
    return (environments ?? []).filter((e) => e.name.toLowerCase().includes(q))
  }, [environments, search])

  const {
    page: definitionsPage,
    setPage: setDefinitionsPage,
    resetPage: resetDefinitionsPage,
    pageCount: definitionsPageCount,
    pageItems: pageDefinitions,
  } = usePagination(filteredEnvironments)
  const { page: instancesPage, setPage: setInstancesPage, pageCount: instancesPageCount, pageItems: pageInstances } =
    usePagination(instances ?? [])

  useEffect(resetDefinitionsPage, [search, resetDefinitionsPage])

  const handleCreate = async (name: string, image: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await createEnvironment({ name, image, mounts: [] })
      setCreateOpen(false)
      reloadEnvironments()
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const handleLaunch = async (name: string) => {
    setLaunching(name)
    setError(null)
    try {
      await launchEnvironment(name)
      reloadInstances()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLaunching(null)
    }
  }

  const handleStop = async (id: string) => {
    setStopping(id)
    setError(null)
    try {
      await stopInstance(id)
      reloadInstances()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setStopping(null)
    }
  }

  const handleDelete = async (name: string) => {
    if (!(await confirm(`Delete environment "${name}"? This cannot be undone.`))) return

    setDeleting(name)
    setError(null)
    try {
      await deleteEnvironment(name)
      showToast(`Deleted environment "${name}"`)
      reloadEnvironments()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setDeleting(null)
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

      {error && <p className="error">{error}</p>}

      <div className="page-header">
        <h3>Definitions</h3>
      </div>

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search environments…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
          <div className="list-toolbar-actions">
            <button
              type="button"
              className="icon-button"
              title="Create a custom environment"
              aria-label="Create a custom environment"
              onClick={() => setCreateOpen(true)}
            >
              <Plus size={16} />
            </button>
          </div>
        </div>

        {!error && environments === null && (
          <div className="panel-body">
            <TableSkeleton columns={5} bare />
          </div>
        )}

        {environments !== null && environments.length === 0 && (
          <div className="panel-body">
            <p className="hint">No environment definitions found.</p>
          </div>
        )}

        {environments !== null && environments.length > 0 && filteredEnvironments.length === 0 && (
          <div className="panel-body">
            <p className="hint">No environments match your search.</p>
          </div>
        )}

        {filteredEnvironments.length > 0 && (
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
              {pageDefinitions.map((env) => (
                <tr key={env.name}>
                  <td>
                    <Link to={`/environments/${encodeURIComponent(env.name)}`}>{env.name}</Link>{' '}
                    {env.prebuilt && <span className="badge">prebuilt</span>}
                  </td>
                  <td>
                    <code>{env.image}</code>
                  </td>
                  <td>{env.tools.length}</td>
                  <td>{env.mounts.length}</td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="icon-button"
                      title="Launch"
                      aria-label="Launch"
                      disabled={launching === env.name}
                      onClick={() => handleLaunch(env.name)}
                    >
                      <Play size={15} />
                    </button>
                    <button
                      type="button"
                      className="icon-button"
                      title="Delete environment"
                      aria-label="Delete environment"
                      disabled={deleting === env.name}
                      onClick={() => handleDelete(env.name)}
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
          page={definitionsPage}
          pageCount={definitionsPageCount}
          onChange={setDefinitionsPage}
          shownCount={filteredEnvironments.length}
          totalCount={environments?.length ?? 0}
          itemLabel="environments"
        />
      </div>

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
              {pageInstances.map((instance) => (
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
                    <button
                      type="button"
                      className="icon-button"
                      title="Stop"
                      aria-label="Stop"
                      disabled={stopping === instance.id}
                      onClick={() => handleStop(instance.id)}
                    >
                      <Square size={15} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pagination
            page={instancesPage}
            pageCount={instancesPageCount}
            onChange={setInstancesPage}
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
        <div className="row-actions confirm-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={creating || !name.trim() || !image.trim()}>
            {creating ? 'Creating…' : 'Create'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

export default Environments
