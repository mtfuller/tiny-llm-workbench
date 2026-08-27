import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { createTool, deleteTool, listTools, updateTool, type Tool } from '../api'
import { useConfirm } from '../ConfirmDialog'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { emptyTool, toDraftTool, toPayloadTool, ToolParameterFields, type DraftTool } from '../ToolEditor'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'

type ModalState = { mode: 'add' } | { mode: 'edit'; tool: Tool } | null

// Tools is the global catalog of runnable tool definitions — a live
// reference model: an Environment attaches a tool by name, so editing a
// tool here changes it everywhere it's attached (see
// project_phase2_architecture.md / CLAUDE.md's Docker orchestration
// decision for the reasoning).
function Tools() {
  const confirm = useConfirm()
  const showToast = useToast()

  const [tools, setTools] = useState<Tool[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)

  const [modal, setModal] = useState<ModalState>(null)
  const [modalSaving, setModalSaving] = useState(false)
  const [modalError, setModalError] = useState<string | null>(null)

  const reload = () => {
    listTools()
      .then(setTools)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(reload, [])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return tools ?? []
    return (tools ?? []).filter((t) => t.name.toLowerCase().includes(q) || (t.description ?? '').toLowerCase().includes(q))
  }, [tools, search])

  const { page, setPage, resetPage, pageCount, pageItems } = usePagination(filtered)
  useEffect(resetPage, [search, resetPage])

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
      reload()
    } catch (err) {
      setModalError((err as Error).message)
    } finally {
      setModalSaving(false)
    }
  }

  const handleDelete = async (tool: Tool) => {
    if (!(await confirm(`Delete tool "${tool.name}"? Any environment that has it attached will lose access to it.`))) return

    setDeleting(tool.name)
    setError(null)
    try {
      await deleteTool(tool.name)
      showToast(`Deleted tool "${tool.name}"`)
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setDeleting(null)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Tools</h2>
      </div>
      <p className="hint">
        A shared catalog of runnable commands. Attach a tool to an Environment (or an Agent's tool node) by name —
        editing it here updates it everywhere it's attached.
      </p>

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search tools…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
          <div className="list-toolbar-actions">
            <button
              type="button"
              className="icon-button"
              title="New tool"
              aria-label="New tool"
              onClick={() => setModal({ mode: 'add' })}
            >
              <Plus size={16} />
            </button>
          </div>
        </div>

        {error && (
          <div className="panel-body">
            <p className="error">{error}</p>
          </div>
        )}

        {!error && tools === null && (
          <div className="panel-body">
            <TableSkeleton columns={5} />
          </div>
        )}

        {tools !== null && tools.length === 0 && (
          <div className="panel-body">
            <p className="hint">No tools yet. Create one above.</p>
          </div>
        )}

        {tools !== null && tools.length > 0 && filtered.length === 0 && (
          <div className="panel-body">
            <p className="hint">No tools match your search.</p>
          </div>
        )}

        {filtered.length > 0 && (
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
                {pageItems.map((tool) => (
                  <tr key={tool.name}>
                    <td>
                      {tool.name} {tool.prebuilt && <span className="badge">prebuilt</span>}
                    </td>
                    <td>{tool.description || '—'}</td>
                    <td>
                      <code>{tool.command}</code>
                    </td>
                    <td>{tool.parameters.length}</td>
                    <td className="row-actions">
                      <button
                        type="button"
                        className="icon-button"
                        title="Edit tool"
                        aria-label="Edit tool"
                        onClick={() => setModal({ mode: 'edit', tool })}
                      >
                        <Pencil size={15} />
                      </button>
                      <button
                        type="button"
                        className="icon-button"
                        title="Delete tool"
                        aria-label="Delete tool"
                        disabled={deleting === tool.name}
                        onClick={() => handleDelete(tool)}
                      >
                        <Trash2 size={15} />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <Pagination
          page={page}
          pageCount={pageCount}
          onChange={setPage}
          shownCount={filtered.length}
          totalCount={tools?.length ?? 0}
          itemLabel="tools"
        />
      </div>

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
          your own quotes around the placeholder.
        </p>

        <div className="mounts-editor">
          <span>Parameters</span>
          <ToolParameterFields parameters={draft.parameters} onChange={(parameters) => setDraft((d) => ({ ...d, parameters }))} />
        </div>

        {error && <p className="error">{error}</p>}
        <div className="row-actions confirm-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={saving || !draft.name.trim() || !draft.command.trim()}>
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

export default Tools
