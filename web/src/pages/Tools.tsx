import { Copy, Pencil, Plus, Square, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import {
  createTool,
  deleteTool,
  getExec,
  listTools,
  listWorkspaces,
  stopInstance,
  tryCatalogTool,
  updateTool,
  type Exec,
  type Tool,
  type Workspace,
} from '../api'
import { Badge } from '../Badge'
import { useEventStream } from '../eventStream'
import IconButton from '../IconButton'
import ListPanel from '../ListPanel'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import TabBar from '../TabBar'
import { emptyTool, toDraftTool, toPayloadTool, ToolParameterFields, type DraftTool } from '../ToolEditor'
import { useToast } from '../Toast'
import { useResourceList } from '../useResourceList'

type ModalState = { mode: 'add' } | { mode: 'edit'; tool: Tool } | null
type Tab = 'catalog' | 'playground'

// Tools is the global catalog of runnable tool definitions — a live
// reference model: an Agent names a tool by name, so editing a tool here
// changes it everywhere it's referenced. The Playground tab runs a tool in
// a test workspace's sandbox so you can see its effects.
function Tools() {
  const showToast = useToast()
  const [tab, setTab] = useState<Tab>('catalog')
  const list = useResourceList<Tool>({
    load: listTools,
    getName: (t) => t.name,
    searchText: (t) => t.description ?? '',
    remove: (t) => deleteTool(t.name),
    confirmMessage: (t) => `Delete tool "${t.name}"? Any agent that references it will lose access to it.`,
    deletedToast: (t) => `Deleted tool "${t.name}"`,
  })

  const [modal, setModal] = useState<ModalState>(null)
  const [modalSaving, setModalSaving] = useState(false)
  const [modalError, setModalError] = useState<string | null>(null)

  const handleSaveModal = async (draft: DraftTool) => {
    if (!modal) return

    setModalSaving(true)
    setModalError(null)
    try {
      const payload = toPayloadTool(draft)
      if (modal.mode === 'add') {
        await createTool(payload)
        showToast('Created tool')
      } else {
        await updateTool(modal.tool.name, payload)
        showToast('Saved tool')
      }
      setModal(null)
      list.reload()
    } catch (err) {
      setModalError((err as Error).message)
    } finally {
      setModalSaving(false)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Tools</h2>
      </div>
      <p className="hint">
        A shared catalog of runnable commands. An agent picks which tools it can use (Agent settings) —
        editing one here updates it everywhere it's referenced.
      </p>

      <TabBar
        tabs={[
          { value: 'catalog', label: 'Catalog' },
          { value: 'playground', label: 'Playground' },
        ]}
        value={tab}
        onChange={setTab}
      />

      {tab === 'catalog' && (
        <ListPanel
          search={list.search}
          onSearch={list.setSearch}
          searchPlaceholder="Search tools…"
          actions={<IconButton icon={<Plus size={16} />} label="New tool" onClick={() => setModal({ mode: 'add' })} />}
          error={list.error}
          loading={list.items === null}
          isEmpty={list.items !== null && list.items.length === 0}
          hasMatches={list.filtered.length > 0}
          emptyMessage="No tools yet. Create one above."
          noMatchMessage="No tools match your search."
          skeletonColumns={5}
          page={list.page}
          pageCount={list.pageCount}
          setPage={list.setPage}
          shownCount={list.filtered.length}
          totalCount={list.items?.length ?? 0}
          itemLabel="tools"
        >
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
                {list.pageItems.map((tool) => (
                  <tr key={tool.name}>
                    <td>
                      {tool.name} {tool.prebuilt && <Badge>prebuilt</Badge>}
                    </td>
                    <td>{tool.description || '—'}</td>
                    <td>
                      <code>{tool.command}</code>
                    </td>
                    <td>{tool.parameters.length}</td>
                    <td className="row-actions">
                      <IconButton icon={<Pencil size={15} />} label="Edit tool" onClick={() => setModal({ mode: 'edit', tool })} />
                      <IconButton
                        icon={<Trash2 size={15} />}
                        label="Delete tool"
                        disabled={list.deleting === tool.name}
                        onClick={() => list.handleDelete(tool)}
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </ListPanel>
      )}

      {tab === 'playground' && <Playground tools={list.items ?? []} />}

      {modal && (
        <ToolModal
          initial={modal.mode === 'edit' ? toDraftTool(modal.tool) : emptyTool()}
          editingName={modal.mode === 'edit'}
          saving={modalSaving}
          error={modalError}
          onSave={handleSaveModal}
          onClose={() => setModal(null)}
        />
      )}
    </>
  )
}

// Playground runs a catalog tool inside a fresh sandbox seeded from a test
// workspace, so you can see what it actually does to the files.
function Playground({ tools }: { tools: Tool[] }) {
  const showToast = useToast()
  const { subscribe } = useEventStream()

  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [toolName, setToolName] = useState('')
  const [workspaceName, setWorkspaceName] = useState('')
  const [args, setArgs] = useState<Record<string, string>>({})
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [instanceId, setInstanceId] = useState<string | null>(null)
  const [workspacePath, setWorkspacePath] = useState<string | null>(null)
  const [activeExec, setActiveExec] = useState<Exec | null>(null)

  useEffect(() => {
    listWorkspaces()
      .then((all) => setWorkspaces(all.filter((w) => w.type === 'test')))
      .catch(() => setWorkspaces([]))
  }, [])

  const selectedTool = useMemo(() => tools.find((t) => t.name === toolName), [tools, toolName])

  // SSE + poll fallback for the running exec (see feedback_sse_needs_poll_fallback).
  useEffect(() => {
    const off1 = subscribe('workspace.exec.output', (event) => {
      const { execId, chunk } = JSON.parse(event.data) as { execId: string; chunk: string }
      setActiveExec((prev) => (prev && prev.id === execId ? { ...prev, output: prev.output + chunk } : prev))
    })
    const off2 = subscribe('workspace.exec.status', (event) => {
      const exec = JSON.parse(event.data) as Exec
      setActiveExec((prev) => (prev && prev.id === exec.id ? exec : prev))
    })
    return () => {
      off1()
      off2()
    }
  }, [subscribe])

  const execId = activeExec?.id
  const execRunning = activeExec?.status === 'running'
  useEffect(() => {
    if (!execRunning || !instanceId || !execId) return
    const interval = setInterval(() => {
      getExec(instanceId, execId)
        .then((exec) => setActiveExec((prev) => (prev && prev.id === exec.id ? exec : prev)))
        .catch(() => {})
    }, 1000)
    return () => clearInterval(interval)
  }, [execRunning, execId, instanceId])

  const handleToolChange = (n: string) => {
    setToolName(n)
    setArgs({})
  }

  const handleRun = async (e: FormEvent) => {
    e.preventDefault()
    if (!toolName || (!instanceId && !workspaceName)) return
    setRunning(true)
    setError(null)
    try {
      const res = await tryCatalogTool(toolName, {
        workspaceName: instanceId ? undefined : workspaceName,
        instanceId: instanceId ?? undefined,
        args,
      })
      setActiveExec(res.exec)
      setInstanceId(res.instanceId)
      if (res.workspacePath) setWorkspacePath(res.workspacePath)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setRunning(false)
    }
  }

  const handleStopSandbox = async () => {
    if (!instanceId) return
    await stopInstance(instanceId).catch(() => {})
    setInstanceId(null)
    setWorkspacePath(null)
    setActiveExec(null)
  }

  const copyPath = () => {
    if (!workspacePath) return
    navigator.clipboard?.writeText(workspacePath).then(
      () => showToast('Path copied'),
      () => showToast('Could not copy'),
    )
  }

  return (
    <div className="panel">
      {tools.length === 0 ? (
        <p className="hint">No tools in the catalog yet — create one on the Catalog tab.</p>
      ) : (
        <form className="stacked-form" onSubmit={handleRun}>
          <label>
            Tool
            <select value={toolName} onChange={(e) => handleToolChange(e.target.value)}>
              <option value="">Select a tool…</option>
              {tools.map((t) => (
                <option key={t.name} value={t.name}>
                  {t.name}
                </option>
              ))}
            </select>
          </label>

          <label>
            Test workspace
            <select value={workspaceName} onChange={(e) => setWorkspaceName(e.target.value)} disabled={!!instanceId}>
              <option value="">Select a test workspace…</option>
              {workspaces.map((w) => (
                <option key={w.name} value={w.name}>
                  {w.name}
                </option>
              ))}
            </select>
            {workspaces.length === 0 && (
              <span className="field-hint">
                No test workspaces yet — create one on the <Link to="/workspaces">Workspaces</Link> page.
              </span>
            )}
          </label>

          {selectedTool && selectedTool.parameters.length > 0 && (
            <div className="mounts-editor">
              <span>Parameters</span>
              {selectedTool.parameters.map((p) => (
                <label key={p.name}>
                  {p.name}
                  {p.required ? ' *' : ''}
                  {p.type === 'boolean' ? (
                    <select value={args[p.name] ?? ''} onChange={(e) => setArgs((a) => ({ ...a, [p.name]: e.target.value }))}>
                      <option value="">—</option>
                      <option value="true">true</option>
                      <option value="false">false</option>
                    </select>
                  ) : (
                    <input
                      type={p.type === 'number' ? 'number' : 'text'}
                      value={args[p.name] ?? ''}
                      onChange={(e) => setArgs((a) => ({ ...a, [p.name]: e.target.value }))}
                    />
                  )}
                  {p.description && <span className="field-hint">{p.description}</span>}
                </label>
              ))}
            </div>
          )}

          {error && <p className="error">{error}</p>}

          <div className="row-actions">
            <button type="submit" disabled={running || !toolName || (!instanceId && !workspaceName)}>
              {running ? 'Running…' : instanceId ? 'Run again' : 'Run'}
            </button>
            {instanceId && (
              <button type="button" onClick={handleStopSandbox}>
                <Square size={14} /> Stop sandbox
              </button>
            )}
          </div>
        </form>
      )}

      {workspacePath && (
        <p className="hint">
          Sandbox folder: <code>{workspacePath}</code>{' '}
          <button type="button" className="icon-button" title="Copy path" aria-label="Copy path" onClick={copyPath}>
            <Copy size={14} />
          </button>{' '}
          — open it in your editor to see the effects.
        </p>
      )}

      {activeExec && (
        <>
          <div className="page-header">
            <h3>
              Output{' '}
              <span
                className={`status ${
                  activeExec.status === 'failed'
                    ? 'status-closed'
                    : activeExec.status === 'done'
                      ? 'status-open'
                      : ''
                }`}
              >
                {activeExec.status}
                {activeExec.exitCode !== undefined ? ` (exit ${activeExec.exitCode})` : ''}
              </span>
            </h3>
          </div>
          <pre className="exec-output">{activeExec.output || '(no output)'}</pre>
          {activeExec.error && <p className="error">{activeExec.error}</p>}
        </>
      )}
    </div>
  )
}

interface ToolModalProps {
  initial: DraftTool
  editingName: boolean
  saving: boolean
  error: string | null
  onSave: (draft: DraftTool) => void
  onClose: () => void
}

function ToolModal({ initial, editingName, saving, error, onSave, onClose }: ToolModalProps) {
  const [draft, setDraft] = useState<DraftTool>(initial)

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!draft.name.trim() || !draft.command.trim()) return
    onSave(draft)
  }

  return (
    <Modal title={editingName ? 'Edit tool' : 'New tool'} onClose={onClose} size="lg">
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input
            type="text"
            value={draft.name}
            onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))}
            disabled={editingName}
            autoFocus={!editingName}
          />
        </label>
        <label>
          Description
          <input type="text" value={draft.description} onChange={(e) => setDraft((d) => ({ ...d, description: e.target.value }))} />
        </label>
        <label>
          Command
          <textarea
            rows={2}
            placeholder="cat {{path}}"
            value={draft.command}
            onChange={(e) => setDraft((d) => ({ ...d, command: e.target.value }))}
          />
        </label>
        <p className="hint">
          Reference a parameter in the command with <code>{'{{paramName}}'}</code> — values are always safely quoted, so don't add
          your own quotes around the placeholder. Paths are relative to <code>/workspace</code>.
        </p>

        <div className="mounts-editor">
          <span>Parameters</span>
          <ToolParameterFields parameters={draft.parameters} onChange={(parameters) => setDraft((d) => ({ ...d, parameters }))} />
        </div>

        {error && <p className="error">{error}</p>}
        <ModalActions onCancel={onClose} busy={saving} disabled={!draft.name.trim() || !draft.command.trim()} />
      </form>
    </Modal>
  )
}

export default Tools
