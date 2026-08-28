import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { createTool, deleteTool, listTools, updateTool, type Tool } from '../api'
import { Badge } from '../Badge'
import IconButton from '../IconButton'
import ListPanel from '../ListPanel'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import { emptyTool, toDraftTool, toPayloadTool, ToolParameterFields, type DraftTool } from '../ToolEditor'
import { useToast } from '../Toast'
import { useResourceList } from '../useResourceList'

type ModalState = { mode: 'add' } | { mode: 'edit'; tool: Tool } | null

// Tools is the global catalog of runnable tool definitions — a live
// reference model: an Environment attaches a tool by name, so editing a
// tool here changes it everywhere it's attached (see
// project_phase2_architecture.md / CLAUDE.md's Docker orchestration
// decision for the reasoning).
function Tools() {
  const showToast = useToast()
  const list = useResourceList<Tool>({
    load: listTools,
    getName: (t) => t.name,
    searchText: (t) => t.description ?? '',
    remove: (t) => deleteTool(t.name),
    confirmMessage: (t) => `Delete tool "${t.name}"? Any environment that has it attached will lose access to it.`,
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
        A shared catalog of runnable commands. Attach a tool to an Environment (or an Agent's tool node) by name —
        editing it here updates it everywhere it's attached.
      </p>

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
        <ModalActions onCancel={onClose} busy={saving} disabled={!draft.name.trim() || !draft.command.trim()} />
      </form>
    </Modal>
  )
}

export default Tools
