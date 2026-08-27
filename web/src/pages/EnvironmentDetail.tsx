import { Link2Off, Plus, Square } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  attachTool,
  detachTool,
  getEnvironment,
  getExec,
  launchEnvironment,
  listInstances,
  listTools,
  stopInstance,
  tryTool,
  updateEnvironmentConfig,
  type Environment,
  type Exec,
  type Instance,
  type Mount,
  type Tool,
} from '../api'
import { useConfirm } from '../ConfirmDialog'
import { useEventStream } from '../eventStream'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'

type Tab = 'configuration' | 'tools' | 'playground'

function EnvironmentDetail() {
  const { name = '' } = useParams<{ name: string }>()
  const { subscribe } = useEventStream()
  const confirm = useConfirm()
  const showToast = useToast()

  const [tab, setTab] = useState<Tab>('configuration')
  const [environment, setEnvironment] = useState<Environment | null>(null)
  const [instances, setInstances] = useState<Instance[] | null>(null)
  const [catalog, setCatalog] = useState<Tool[]>([])
  const [error, setError] = useState<string | null>(null)

  const [image, setImage] = useState('')
  const [mounts, setMounts] = useState<Mount[]>([])
  const [savingConfig, setSavingConfig] = useState(false)
  const [configError, setConfigError] = useState<string | null>(null)

  const [toolSearch, setToolSearch] = useState('')
  const [attachOpen, setAttachOpen] = useState(false)
  const [attaching, setAttaching] = useState(false)
  const [attachError, setAttachError] = useState<string | null>(null)
  const [detachingTool, setDetachingTool] = useState<string | null>(null)

  const [launching, setLaunching] = useState(false)
  const [stopping, setStopping] = useState<string | null>(null)
  const [playgroundInstanceId, setPlaygroundInstanceId] = useState('')
  const [playgroundToolName, setPlaygroundToolName] = useState('')
  const [playgroundArgs, setPlaygroundArgs] = useState<Record<string, string>>({})
  const [running, setRunning] = useState(false)
  const [activeExec, setActiveExec] = useState<Exec | null>(null)

  const reloadEnvironment = useCallback(() => {
    getEnvironment(name)
      .then((env) => {
        setEnvironment(env)
        setImage(env.image)
        setMounts(env.mounts)
      })
      .catch((err: Error) => setError(err.message))
  }, [name])

  const reloadInstances = useCallback(() => {
    listInstances()
      .then(setInstances)
      .catch((err: Error) => setError(err.message))
  }, [])

  const reloadCatalog = useCallback(() => {
    listTools()
      .then(setCatalog)
      .catch((err: Error) => setError(err.message))
  }, [])

  useEffect(() => {
    reloadEnvironment()
    reloadInstances()
    reloadCatalog()
  }, [reloadEnvironment, reloadInstances, reloadCatalog])

  // Poll instances while this page is open, since launching/stopping an
  // instance elsewhere (the Environments list page) doesn't otherwise
  // notify this one.
  useEffect(() => {
    const interval = setInterval(reloadInstances, 4000)
    return () => clearInterval(interval)
  }, [reloadInstances])

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

  // A tool that finishes fast (most of them do) can race the SSE "done"
  // event ahead of the tryTool response the UI sets activeExec from — the
  // same lesson as Training/Evaluations/Benchmarks (see
  // feedback_sse_needs_poll_fallback), just applied here for the first
  // time. Poll as a fallback while an exec is running so the UI always
  // reconciles even if the SSE event arrived too early to be seen.
  useEffect(() => {
    if (!activeExec || activeExec.status !== 'running') return
    const interval = setInterval(() => {
      getExec(activeExec.instanceId, activeExec.id)
        .then((exec) => setActiveExec((prev) => (prev && prev.id === exec.id ? exec : prev)))
        .catch(() => {})
    }, 1000)
    return () => clearInterval(interval)
  }, [activeExec])

  const myInstances = useMemo(() => (instances ?? []).filter((i) => i.environmentName === name), [instances, name])

  // Attached tools are resolved by joining the environment's name references
  // against the global catalog — a name that no longer resolves (the
  // catalog entry was deleted elsewhere) is silently dropped here, the same
  // graceful-degradation behavior the backend uses when actually running a
  // tool node.
  const attachedNames = environment?.tools ?? []
  const tools = useMemo(
    () => attachedNames.map((n) => catalog.find((t) => t.name === n)).filter((t): t is Tool => t !== undefined),
    [attachedNames, catalog],
  )
  const unattachedCatalog = useMemo(() => catalog.filter((t) => !attachedNames.includes(t.name)), [catalog, attachedNames])

  const filteredTools = useMemo(() => {
    const q = toolSearch.trim().toLowerCase()
    return tools.filter((t) => !q || t.name.toLowerCase().includes(q) || (t.description ?? '').toLowerCase().includes(q))
  }, [tools, toolSearch])

  const {
    page: toolPage,
    setPage: setToolPage,
    resetPage: resetToolPage,
    pageCount: toolPageCount,
    pageItems: toolPageItems,
  } = usePagination(filteredTools)
  useEffect(resetToolPage, [toolSearch, resetToolPage])

  const {
    page: instancePage,
    setPage: setInstancePage,
    pageCount: instancePageCount,
    pageItems: instancePageItems,
  } = usePagination(myInstances)

  const handleSaveConfig = async (e: FormEvent) => {
    e.preventDefault()
    setSavingConfig(true)
    setConfigError(null)
    try {
      const cleanedMounts = mounts.filter((m) => m.hostPath.trim() && m.containerPath.trim())
      const env = await updateEnvironmentConfig(name, image.trim(), cleanedMounts)
      setEnvironment(env)
      setMounts(env.mounts)
      showToast('Saved configuration')
    } catch (err) {
      setConfigError((err as Error).message)
    } finally {
      setSavingConfig(false)
    }
  }

  const addMountRow = () => setMounts((prev) => [...prev, { hostPath: '', containerPath: '', readOnly: false }])
  const updateMountRow = (index: number, patch: Partial<Mount>) => {
    setMounts((prev) => prev.map((m, i) => (i === index ? { ...m, ...patch } : m)))
  }
  const removeMountRow = (index: number) => setMounts((prev) => prev.filter((_, i) => i !== index))

  const handleAttachTool = async (toolName: string) => {
    setAttaching(true)
    setAttachError(null)
    try {
      const env = await attachTool(name, toolName)
      setEnvironment(env)
      showToast(`Attached "${toolName}"`)
      setAttachOpen(false)
    } catch (err) {
      setAttachError((err as Error).message)
    } finally {
      setAttaching(false)
    }
  }

  const handleDetachTool = async (toolName: string) => {
    if (!(await confirm(`Detach "${toolName}" from this environment? The tool itself stays in the catalog.`))) return

    setDetachingTool(toolName)
    setError(null)
    try {
      await detachTool(name, toolName)
      showToast(`Detached "${toolName}"`)
      reloadEnvironment()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setDetachingTool(null)
    }
  }

  const handleLaunch = async () => {
    setLaunching(true)
    setError(null)
    try {
      const instance = await launchEnvironment(name)
      reloadInstances()
      setPlaygroundInstanceId(instance.id)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLaunching(false)
    }
  }

  const handleStop = async (id: string) => {
    setStopping(id)
    setError(null)
    try {
      await stopInstance(id)
      reloadInstances()
      if (playgroundInstanceId === id) setPlaygroundInstanceId('')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setStopping(null)
    }
  }

  const selectedTool: Tool | null = tools.find((t) => t.name === playgroundToolName) ?? null

  const handleToolNameChange = (value: string) => {
    setPlaygroundToolName(value)
    setPlaygroundArgs({})
  }

  const handleRunTool = async (e: FormEvent) => {
    e.preventDefault()
    if (!playgroundInstanceId || !playgroundToolName) return

    setRunning(true)
    setError(null)
    try {
      const exec = await tryTool(name, playgroundToolName, playgroundInstanceId, playgroundArgs)
      setActiveExec(exec)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setRunning(false)
    }
  }

  if (!environment && !error) {
    return (
      <>
        <div className="page-header">
          <h2>
            <Link to="/environments">Environments</Link> / {name}
          </h2>
        </div>
        <TableSkeleton columns={4} />
      </>
    )
  }

  return (
    <>
      <div className="page-header">
        <h2>
          <Link to="/environments">Environments</Link> / {name}
          {environment?.prebuilt && <span className="badge">prebuilt</span>}
        </h2>
      </div>

      {error && <p className="error">{error}</p>}

      {environment && (
        <section className="panel">
          <h3>Environment info</h3>
          <dl className="info-list">
            <dt>Image</dt>
            <dd>
              <code>{environment.image}</code>
            </dd>
            <dt>Tools</dt>
            <dd>{environment.tools.length}</dd>
            <dt>Mounts</dt>
            <dd>{environment.mounts.length}</dd>
            <dt>Created</dt>
            <dd>{new Date(environment.createdAt).toLocaleString()}</dd>
          </dl>
        </section>
      )}

      <div className="tab-bar">
        <button
          type="button"
          className={`tab-button${tab === 'configuration' ? ' tab-button-active' : ''}`}
          onClick={() => setTab('configuration')}
        >
          Configuration
        </button>
        <button type="button" className={`tab-button${tab === 'tools' ? ' tab-button-active' : ''}`} onClick={() => setTab('tools')}>
          Tools
        </button>
        <button
          type="button"
          className={`tab-button${tab === 'playground' ? ' tab-button-active' : ''}`}
          onClick={() => setTab('playground')}
        >
          Playground
        </button>
      </div>

      {tab === 'configuration' && (
        <section className="panel">
          <form className="stacked-form" onSubmit={handleSaveConfig}>
            <label>
              Docker image
              <input type="text" value={image} onChange={(e) => setImage(e.target.value)} />
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
                  <label className="checkbox-label">
                    <input
                      type="checkbox"
                      checked={mount.readOnly ?? false}
                      onChange={(e) => updateMountRow(i, { readOnly: e.target.checked })}
                    />
                    read-only
                  </label>
                  <button type="button" className="danger-button" onClick={() => removeMountRow(i)}>
                    ×
                  </button>
                </div>
              ))}
              <button type="button" className="button-secondary" onClick={addMountRow}>
                + Mount
              </button>
            </div>

            {configError && <p className="error">{configError}</p>}
            <div className="row-actions confirm-actions">
              <button type="submit" disabled={savingConfig || !image.trim()}>
                {savingConfig ? 'Saving…' : 'Save configuration'}
              </button>
            </div>
          </form>
        </section>
      )}

      {tab === 'tools' && (
        <div className="panel panel-flush">
          <div className="list-toolbar panel-toolbar">
            <input
              type="search"
              placeholder="Search tools…"
              value={toolSearch}
              onChange={(e) => setToolSearch(e.target.value)}
              className="list-search"
            />
            <div className="list-toolbar-actions">
              <button
                type="button"
                className="icon-button"
                title="Attach tool"
                aria-label="Attach tool"
                onClick={() => setAttachOpen(true)}
              >
                <Plus size={16} />
              </button>
            </div>
          </div>

          {tools.length === 0 && (
            <div className="panel-body">
              <p className="hint">
                No tools attached yet. Attach one from the catalog to give this environment something an agent (or you, in the
                Playground) can run.
              </p>
            </div>
          )}

          {tools.length > 0 && filteredTools.length === 0 && (
            <div className="panel-body">
              <p className="hint">No tools match your search.</p>
            </div>
          )}

          {filteredTools.length > 0 && (
            <div className="table-scroll">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Description</th>
                    <th>Command</th>
                    <th>Parameters</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {toolPageItems.map((t) => (
                    <tr key={t.name}>
                      <td>
                        <Link to="/tools">{t.name}</Link>
                      </td>
                      <td>{t.description || '—'}</td>
                      <td>
                        <code>{t.command}</code>
                      </td>
                      <td>{t.parameters.length}</td>
                      <td className="row-actions">
                        <button
                          type="button"
                          className="icon-button"
                          title="Detach tool"
                          aria-label="Detach tool"
                          disabled={detachingTool === t.name}
                          onClick={() => handleDetachTool(t.name)}
                        >
                          <Link2Off size={15} />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <Pagination
            page={toolPage}
            pageCount={toolPageCount}
            onChange={setToolPage}
            shownCount={filteredTools.length}
            totalCount={tools.length}
            itemLabel="tools"
          />
        </div>
      )}

      {tab === 'playground' && (
        <>
          <section className="panel">
            <h3>Instances</h3>
            <p className="hint">Launch a fresh container from this environment, or reuse one already running.</p>
            <div className="row-actions confirm-actions">
              <button type="button" onClick={handleLaunch} disabled={launching}>
                {launching ? 'Launching…' : 'Launch new instance'}
              </button>
            </div>

            {myInstances.length === 0 && <p className="empty-state">No instances of this environment running.</p>}

            {myInstances.length > 0 && (
              <div className="panel panel-flush">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>State</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {instancePageItems.map((instance) => (
                      <tr key={instance.id}>
                        <td>{instance.name}</td>
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
                  page={instancePage}
                  pageCount={instancePageCount}
                  onChange={setInstancePage}
                  shownCount={myInstances.length}
                  totalCount={myInstances.length}
                  itemLabel="instances"
                />
              </div>
            )}
          </section>

          <section className="panel">
            <h3>Try a tool</h3>
            {tools.length === 0 ? (
              <p className="hint">Attach a tool in the Tools tab first.</p>
            ) : (
              <form className="stacked-form" onSubmit={handleRunTool}>
                <label>
                  Instance
                  <select value={playgroundInstanceId} onChange={(e) => setPlaygroundInstanceId(e.target.value)}>
                    <option value="">Select a running instance…</option>
                    {myInstances.map((instance) => (
                      <option key={instance.id} value={instance.id}>
                        {instance.name} ({instance.id.slice(0, 12)})
                      </option>
                    ))}
                  </select>
                </label>

                <label>
                  Tool
                  <select value={playgroundToolName} onChange={(e) => handleToolNameChange(e.target.value)}>
                    <option value="">Select a tool…</option>
                    {tools.map((t) => (
                      <option key={t.name} value={t.name}>
                        {t.name}
                      </option>
                    ))}
                  </select>
                </label>

                {selectedTool && selectedTool.parameters.length > 0 && (
                  <div className="mounts-editor">
                    <span>Parameters</span>
                    {selectedTool.parameters.map((p) => (
                      <label key={p.name}>
                        {p.name}
                        {p.required ? ' *' : ''}
                        {p.type === 'boolean' ? (
                          <select
                            value={playgroundArgs[p.name] ?? ''}
                            onChange={(e) => setPlaygroundArgs((prev) => ({ ...prev, [p.name]: e.target.value }))}
                          >
                            <option value="">—</option>
                            <option value="true">true</option>
                            <option value="false">false</option>
                          </select>
                        ) : (
                          <input
                            type={p.type === 'number' ? 'number' : 'text'}
                            placeholder={p.description || p.name}
                            value={playgroundArgs[p.name] ?? ''}
                            onChange={(e) => setPlaygroundArgs((prev) => ({ ...prev, [p.name]: e.target.value }))}
                          />
                        )}
                      </label>
                    ))}
                  </div>
                )}

                <div className="row-actions confirm-actions">
                  <button type="submit" disabled={running || !playgroundInstanceId || !playgroundToolName}>
                    {running ? 'Running…' : 'Run'}
                  </button>
                </div>
              </form>
            )}

            {activeExec && (
              <>
                <div className="page-header">
                  <span
                    className={`status ${
                      activeExec.status === 'failed' ? 'status-closed' : activeExec.status === 'done' ? 'status-open' : 'status-connecting'
                    }`}
                  >
                    {activeExec.status}
                    {activeExec.exitCode !== undefined ? ` (exit ${activeExec.exitCode})` : ''}
                  </span>
                </div>
                <pre className="exec-output">{activeExec.output || ' '}</pre>
                {activeExec.error && <p className="error">{activeExec.error}</p>}
              </>
            )}
          </section>
        </>
      )}

      {attachOpen && (
        <AttachToolModal
          options={unattachedCatalog}
          attaching={attaching}
          error={attachError}
          onAttach={handleAttachTool}
          onClose={() => setAttachOpen(false)}
        />
      )}
    </>
  )
}

interface AttachToolModalProps {
  options: Tool[]
  attaching: boolean
  error: string | null
  onAttach: (toolName: string) => void
  onClose: () => void
}

// AttachToolModal lets the user pick an existing catalog tool to reference
// from this environment — a live reference, not a copy (see the Tool
// catalog "live reference" decision in CLAUDE.md). Creating a brand-new
// tool happens on the Tools catalog page instead of here.
function AttachToolModal({ options, attaching, error, onAttach, onClose }: AttachToolModalProps) {
  const [selected, setSelected] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!selected) return
    onAttach(selected)
  }

  return (
    <Modal title="Attach tool" onClose={onClose}>
      {options.length === 0 ? (
        <p className="hint">
          Every catalog tool is already attached, or the catalog is empty. Create a new one on the <Link to="/tools">Tools</Link>{' '}
          page first.
        </p>
      ) : (
        <form className="stacked-form" onSubmit={handleSubmit}>
          <label>
            Tool
            <select value={selected} onChange={(e) => setSelected(e.target.value)} autoFocus>
              <option value="">Select a tool…</option>
              {options.map((t) => (
                <option key={t.name} value={t.name}>
                  {t.name}
                </option>
              ))}
            </select>
          </label>
          <p className="hint">
            Don't see the tool you want? Create it on the <Link to="/tools">Tools</Link> page, then come back here.
          </p>

          {error && <p className="error">{error}</p>}
          <div className="row-actions confirm-actions">
            <button type="button" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" disabled={attaching || !selected}>
              {attaching ? 'Attaching…' : 'Attach'}
            </button>
          </div>
        </form>
      )}
    </Modal>
  )
}

export default EnvironmentDetail
