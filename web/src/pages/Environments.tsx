import { useEffect, useState } from 'react'
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
import { useEventStream } from '../eventStream'

function Environments() {
  const { subscribe } = useEventStream()

  const [environments, setEnvironments] = useState<Environment[] | null>(null)
  const [instances, setInstances] = useState<Instance[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  const [newName, setNewName] = useState('')
  const [newImage, setNewImage] = useState('')
  const [newTools, setNewTools] = useState('')
  const [newMounts, setNewMounts] = useState<Mount[]>([])
  const [creating, setCreating] = useState(false)

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

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newName.trim() || !newImage.trim()) return

    setCreating(true)
    setError(null)
    try {
      await createEnvironment({
        name: newName.trim(),
        image: newImage.trim(),
        tools: newTools
          .split(',')
          .map((t) => t.trim())
          .filter(Boolean),
        mounts: newMounts.filter((m) => m.hostPath.trim() && m.containerPath.trim()),
      })
      setNewName('')
      setNewImage('')
      setNewTools('')
      setNewMounts([])
      reloadEnvironments()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const addMountRow = () => setNewMounts((prev) => [...prev, { hostPath: '', containerPath: '' }])

  const updateMountRow = (index: number, patch: Partial<Mount>) => {
    setNewMounts((prev) => prev.map((m, i) => (i === index ? { ...m, ...patch } : m)))
  }

  const removeMountRow = (index: number) => {
    setNewMounts((prev) => prev.filter((_, i) => i !== index))
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
    if (!window.confirm(`Delete environment "${name}"? This cannot be undone.`)) return

    setDeleting(name)
    setError(null)
    try {
      await deleteEnvironment(name)
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

      <section className="panel">
        <h3>Definitions</h3>
        {environments === null && <p className="hint">Loading…</p>}
        {environments !== null && environments.length === 0 && (
          <p className="empty-state">No environment definitions found.</p>
        )}
        {environments !== null && environments.length > 0 && (
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
              {environments.map((env) => (
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
                    <button type="button" disabled={launching === env.name} onClick={() => handleLaunch(env.name)}>
                      {launching === env.name ? 'Launching…' : 'Launch'}
                    </button>
                    <button
                      type="button"
                      className="danger-button"
                      disabled={deleting === env.name}
                      onClick={() => handleDelete(env.name)}
                    >
                      {deleting === env.name ? 'Deleting…' : 'Delete'}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="panel">
        <h3>Create a custom environment</h3>
        <form className="stacked-form" onSubmit={handleCreate}>
          <label>
            Name
            <input type="text" value={newName} onChange={(e) => setNewName(e.target.value)} />
          </label>
          <label>
            Docker image
            <input type="text" placeholder="alpine:3.20" value={newImage} onChange={(e) => setNewImage(e.target.value)} />
          </label>
          <label>
            Tools (comma-separated)
            <input type="text" placeholder="shell, python" value={newTools} onChange={(e) => setNewTools(e.target.value)} />
          </label>

          <div className="mounts-editor">
            <span>Mounts (host ↔ container)</span>
            {newMounts.map((mount, i) => (
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
            <button type="button" onClick={addMountRow}>
              + Mount
            </button>
          </div>

          <button type="submit" disabled={creating || !newName.trim() || !newImage.trim()}>
            {creating ? 'Creating…' : 'Create environment'}
          </button>
        </form>
      </section>

      <div className="page-header">
        <h3>Running instances</h3>
      </div>

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
                      onClick={() => {
                        setExecInstanceId(instance.id)
                        setActiveExec(null)
                      }}
                    >
                      Exec
                    </button>
                    <button type="button" disabled={stopping === instance.id} onClick={() => handleStop(instance.id)}>
                      {stopping === instance.id ? 'Stopping…' : 'Stop'}
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
    </>
  )
}

export default Environments
