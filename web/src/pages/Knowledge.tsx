import { Plus, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { createKnowledgeBase, deleteKnowledgeBase, listKnowledgeBases, type KnowledgeBase } from '../api'
import IconButton from '../IconButton'
import { formatDateTime } from '../lib/format'
import ListPanel from '../ListPanel'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import { useToast } from '../Toast'
import { useResourceList } from '../useResourceList'

// Knowledge is the list of KnowledgeBases — independent of any Environment
// or Docker container (see the "Knowledge binding" decision in CLAUDE.md):
// a base is just a set of records an Agent's "knowledge" node queries
// directly via deterministic keyword matching.
function Knowledge() {
  const showToast = useToast()
  const list = useResourceList<KnowledgeBase>({
    load: listKnowledgeBases,
    getName: (b) => b.name,
    searchText: (b) => b.description ?? '',
    remove: (b) => deleteKnowledgeBase(b.name),
    confirmMessage: (b) => `Delete knowledge base "${b.name}"? This cannot be undone.`,
    deletedToast: (b) => `Deleted knowledge base "${b.name}"`,
  })

  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const handleCreate = async (name: string, description: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await createKnowledgeBase(name, description || undefined)
      setCreateOpen(false)
      showToast(`Created knowledge base "${name}"`)
      list.reload()
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Knowledge</h2>
      </div>
      <p className="hint">
        Records an agent's "knowledge" node can query — a deterministic keyword match against title and content, not
        embeddings or a vector store.
      </p>

      <ListPanel
        search={list.search}
        onSearch={list.setSearch}
        searchPlaceholder="Search knowledge bases…"
        actions={<IconButton icon={<Plus size={16} />} label="New knowledge base" onClick={() => setCreateOpen(true)} />}
        error={list.error}
        loading={list.items === null}
        isEmpty={list.items !== null && list.items.length === 0}
        hasMatches={list.filtered.length > 0}
        emptyMessage="No knowledge bases yet. Create one above."
        noMatchMessage="No knowledge bases match your search."
        skeletonColumns={5}
        page={list.page}
        pageCount={list.pageCount}
        setPage={list.setPage}
        shownCount={list.filtered.length}
        totalCount={list.items?.length ?? 0}
        itemLabel="knowledge bases"
      >
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Description</th>
              <th>Records</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.pageItems.map((base) => (
              <tr key={base.name}>
                <td>
                  <Link to={`/knowledge/${encodeURIComponent(base.name)}`}>{base.name}</Link>
                </td>
                <td>{base.description || '—'}</td>
                <td>{base.records.length}</td>
                <td>{formatDateTime(base.createdAt)}</td>
                <td className="row-actions">
                  <IconButton
                    icon={<Trash2 size={15} />}
                    label="Delete knowledge base"
                    disabled={list.deleting === base.name}
                    onClick={() => list.handleDelete(base)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ListPanel>

      {createOpen && (
        <CreateKnowledgeBaseModal creating={creating} error={createError} onCreate={handleCreate} onClose={() => setCreateOpen(false)} />
      )}
    </>
  )
}

interface CreateKnowledgeBaseModalProps {
  creating: boolean
  error: string | null
  onCreate: (name: string, description: string) => void
  onClose: () => void
}

function CreateKnowledgeBaseModal({ creating, error, onCreate, onClose }: CreateKnowledgeBaseModalProps) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    onCreate(name.trim(), description.trim())
  }

  return (
    <Modal title="New knowledge base" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </label>
        <label>
          Description (optional)
          <input type="text" value={description} onChange={(e) => setDescription(e.target.value)} />
        </label>
        <p className="hint">You'll add records on the knowledge base's own page next.</p>

        {error && <p className="error">{error}</p>}
        <ModalActions onCancel={onClose} submitLabel="Create" busyLabel="Creating…" busy={creating} disabled={!name.trim()} />
      </form>
    </Modal>
  )
}

export default Knowledge
