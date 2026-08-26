import { Play, Plus, Square, Terminal, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import {
  createEnvironment,
  deleteEnvironment,
  launchEnvironment,
  listEnvironments,
  listInstances,
  startExec,
  stopInstance,
  type Environment,
  type Exec,
  type Instance,
  type Mount,
} from '../api'
import { useConfirm } from '../ConfirmDialog'
import { useEventStream } from '../eventStream'
import Modal from '../Modal'
import { TableSkeleton } from '../Skeleton'
import { useToast } from '../Toast'

function Environments() {
  const { subscribe } = useEventStream()
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

  const [execInstanceId, setExecInstanceId] = useState<string | null>(null)
  const [execCommand, setExecCommand] = useState('')
  const [activeExec, setActiveExec] = useState<Exec | null>(null)

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

  // Poll instances while an exec is running, since starting/stopping
  // instances elsewhere doesn't otherwise notify this page.
  useEffect(() => {
    const interval = setInterval(reloadInstances, 4000)
    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    const unsubscribeOutput = subscribe('environment.exec.output', (event) => {
      const { execId, chunk } = JSON.parse(event.data) as { execId: string; chunk: string }
      setActiveExec((prev) => (prev && prev.id === execId ? { ...prev, output: prev.output + chunk } : prev))
    })

    const unsubscribeStatus = subscribe('environment.exec.status', (event) => {
      const exec = JSON.parse(event.data) as Exec
      setActiveExec((prev) => (prev && prev.id === exec.id ? exec : prev))
    })

    return () => {
      unsubscribeOutput()
      unsubscribeStatus()
    }
  }, [subscribe])

  const filteredEnvironments = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return environments ?? []
    return (environments ?? []).filter((e) => e.name.toLowerCase().includes(q))
  }, [environments, search])

  const handleCreate = async (name: string, image: string, tools: string, mounts: Mount[]) => {
    setCreating(true)
    setCreateError(null)
    try {
      await createEnvironment({
        name,
        image,
        tools: tools
          .split(',')
          .map((t) => t.trim())
          .filter(Boolean),
        mounts: mounts.filter((m) => m.hostPath.trim() && m.containerPath.trim()),
      })
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
      if (execInstanceId === id) {
        setExecInstanceId(null)
        setActiveExec(null)
      }
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

  const handleRunExec = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!execInstanceId || !execCommand.trim()) return

    setError(null)
    try {
      const exec = await startExec(execInstanceId, execCommand.trim())
      setActiveExec(exec)
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Environments</h2>
      </div>
      <p className="hint">
        Sandboxed Docker containers agents will use to do real work in a future phase. Launch one below
        and try a command in it now.
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
              {filteredEnvironments.map((env) => (
                <tr key={env.name}>
                  <td>
                    {env.name} {env.prebuilt && <span className="badge">prebuilt</span>}
                  </td>
                  <td>
                    <code>{env.image}</code>
                  </td>
                  <td>{env.tools.join(', ') || '—'}</td>
                  <td>
                    {env.mounts.length === 0
                      ? '—'
                      : env.mounts.map((m, i) => (
                          <div key={i}>
                            <code>
                              {m.hostPath} → {m.containerPath}
                            </code>
                          </div>
                        ))}
                  </td>
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
              {instances.map((instance) => (
                <tr key={instance.id}>
                  <td>{instance.name}</td>
                  <td>{instance.environmentName}</td>
                  <td>
                    <span className={`status ${instance.state === 'running' ? 'status-open' : 'status-closed'}`}>
                      {instance.state}
                    </span>
                  </td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="icon-button"
                      title="Run a command"
                      aria-label="Run a command"
                      onClick={() => {
                        setExecInstanceId(instance.id)
                        setActiveExec(null)
                      }}
                    >
                      <Terminal size={15} />
                    </button>
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
        </div>
      )}

      {execInstanceId && (
        <section className="panel">
          <h3>Run a command</h3>
          <p className="hint">
            Runs in instance <code>{execInstanceId.slice(0, 12)}</code> via <code>sh -c</code>.
          </p>
          <form className="inline-form" onSubmit={handleRunExec}>
            <input
              type="text"
              placeholder="echo hello"
              value={execCommand}
              onChange={(e) => setExecCommand(e.target.value)}
            />
            <button type="submit" disabled={activeExec?.status === 'running' || !execCommand.trim()}>
              Run
            </button>
          </form>

          {activeExec && (
            <>
              <div className="page-header">
                <span className={`status ${activeExec.status === 'failed' ? 'status-closed' : activeExec.status === 'done' ? 'status-open' : 'status-connecting'}`}>
                  {activeExec.status}
                  {activeExec.exitCode !== undefined ? ` (exit ${activeExec.exitCode})` : ''}
                </span>
              </div>
              <pre className="exec-output">{activeExec.output || ' '}</pre>
              {activeExec.error && <p className="error">{activeExec.error}</p>}
            </>
          )}
        </section>
      )}

      {createOpen && (
        <CreateEnvironmentModal
          creating={creating}
          error={createError}
          onCreate={handleCreate}
          onClose={() => setCreateOpen(false)}
        />
      )}
    </>
  )
}

interface CreateEnvironmentModalProps {
  creating: boolean
  error: string | null
  onCreate: (name: string, image: string, tools: string, mounts: Mount[]) => void
  onClose: () => void
}

function CreateEnvironmentModal({ creating, error, onCreate, onClose }: CreateEnvironmentModalProps) {
  const [name, setName] = useState('')
  const [image, setImage] = useState('')
  const [tools, setTools] = useState('')
  const [mounts, setMounts] = useState<Mount[]>([])

  const addMountRow = () => setMounts((prev) => [...prev, { hostPath: '', containerPath: '' }])

  const updateMountRow = (index: number, patch: Partial<Mount>) => {
    setMounts((prev) => prev.map((m, i) => (i === index ? { ...m, ...patch } : m)))
  }

  const removeMountRow = (index: number) => {
    setMounts((prev) => prev.filter((_, i) => i !== index))
  }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !image.trim()) return
    onCreate(name.trim(), image.trim(), tools, mounts)
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
        <label>
          Tools (comma-separated)
          <input type="text" placeholder="shell, python" value={tools} onChange={(e) => setTools(e.target.value)} />
        </label>

        <div className="mounts-editor">
          <span>Mounts (host ↔ container)</span>
          {mounts.map((mount, i) => (
            <div className="mount-row" key={i}>
              <input
                type="text"
                placeholder="/host/path"
                value={mount.hostPath}
                onChange={(e) => updateMountRow(i, { hostPath: e.target.value })}
              />
              <span className="mount-arrow">→</span>
              <input
                type="text"
                placeholder="/container/path"
                value={mount.containerPath}
                onChange={(e) => updateMountRow(i, { containerPath: e.target.value })}
              />
              <button type="button" className="danger-button" onClick={() => removeMountRow(i)}>
                ×
              </button>
            </div>
          ))}
          <button type="button" className="button-secondary" onClick={addMountRow}>
            + Mount
          </button>
        </div>

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
