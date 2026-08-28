import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { addKnowledgeRecords, deleteKnowledgeRecord, getKnowledgeBase, updateKnowledgeRecord, type KnowledgeBase, type KnowledgeRecord } from '../api'
import { useConfirm } from '../ConfirmDialog'
import FilterMenu from '../FilterMenu'
import IconButton from '../IconButton'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import TagCell from '../TagCell'
import TagInput from '../TagInput'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'
import { useTagFilter } from '../useTagFilter'

const emptyRecord = { title: '', content: '', tags: [] as string[] }

type ModalState = { mode: 'add' } | { mode: 'edit'; index: number } | null

function KnowledgeDetail() {
  const confirm = useConfirm()
  const showToast = useToast()
  const { name = '' } = useParams<{ name: string }>()
  const [base, setBase] = useState<KnowledgeBase | null>(null)
  const [error, setError] = useState<string | null>(null)

  const [search, setSearch] = useState('')

  const [modal, setModal] = useState<ModalState>(null)
  const [modalSaving, setModalSaving] = useState(false)
  const [modalError, setModalError] = useState<string | null>(null)
  const [deletingIndex, setDeletingIndex] = useState<number | null>(null)

  const reload = () => {
    getKnowledgeBase(name)
      .then(setBase)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(reload, [name])

  const records = base?.records ?? []

  const { allTags, activeTags, toggleTag, clearTags, matchesTags } = useTagFilter(records, (r) => r.tags)

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return records
      .map((record, index) => ({ record, index }))
      .filter(({ record }) => {
        if (!matchesTags(record)) return false
        if (q) {
          const haystack = `${record.title} ${record.content}`.toLowerCase()
          if (!haystack.includes(q)) return false
        }
        return true
      })
  }, [records, search, matchesTags])

  const { page, setPage, resetPage, pageCount, pageItems } = usePagination(filtered)
  useEffect(resetPage, [search, activeTags, resetPage])

  const handleSaveModal = async (record: { title: string; content: string; tags?: string[] }) => {
    if (!modal) return

    setModalSaving(true)
    setModalError(null)
    try {
      if (modal.mode === 'add') {
        await addKnowledgeRecords(name, [record])
        showToast('Added record')
      } else {
        await updateKnowledgeRecord(name, modal.index, record)
        showToast('Saved record')
      }
      setModal(null)
      reload()
    } catch (err) {
      setModalError((err as Error).message)
    } finally {
      setModalSaving(false)
    }
  }

  const handleDeleteRecord = async (index: number) => {
    if (!(await confirm('Delete this record? This cannot be undone.'))) return

    setDeletingIndex(index)
    setError(null)
    try {
      await deleteKnowledgeRecord(name, index)
      showToast('Deleted record')
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setDeletingIndex(null)
    }
  }

  if (!base && !error) {
    return (
      <>
        <div className="page-header">
          <h2>
            <Link to="/knowledge">Knowledge</Link> / {name}
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
          <Link to="/knowledge">Knowledge</Link> / {name}
        </h2>
      </div>
      {base?.description && <p className="hint">{base.description}</p>}

      {error && <p className="error">{error}</p>}

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search records…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
          {allTags.length > 0 && (
            <FilterMenu
              groups={[{ key: 'tags', title: 'Tags', options: allTags, active: activeTags, onToggle: toggleTag }]}
              onClearAll={clearTags}
            />
          )}
          <div className="list-toolbar-actions">
            <IconButton icon={<Plus size={16} />} label="Add record" onClick={() => setModal({ mode: 'add' })} />
          </div>
        </div>

        {base !== null && records.length === 0 && (
          <div className="panel-body">
            <p className="hint">No records yet. Add one above.</p>
          </div>
        )}

        {base !== null && records.length > 0 && filtered.length === 0 && (
          <div className="panel-body">
            <p className="hint">No records match your search/filter.</p>
          </div>
        )}

        {filtered.length > 0 && (
          <table className="data-table">
            <thead>
              <tr>
                <th>Title</th>
                <th>Content</th>
                <th>Tags</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {pageItems.map(({ record, index }) => (
                <tr key={record.id || index}>
                  <td className="cell-truncate">{record.title}</td>
                  <td className="cell-truncate">{record.content}</td>
                  <td>
                    <TagCell tags={record.tags} />
                  </td>
                  <td className="row-actions">
                    <IconButton icon={<Pencil size={15} />} label="Edit record" onClick={() => setModal({ mode: 'edit', index })} />
                    <IconButton
                      icon={<Trash2 size={15} />}
                      label="Delete record"
                      disabled={deletingIndex === index}
                      onClick={() => handleDeleteRecord(index)}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        <Pagination
          page={page}
          pageCount={pageCount}
          onChange={setPage}
          shownCount={filtered.length}
          totalCount={records.length}
          itemLabel="records"
        />
      </div>

      {modal && (
        <RecordModal
          title={modal.mode === 'add' ? 'Add record' : 'Edit record'}
          initial={modal.mode === 'edit' ? records[modal.index] : emptyRecord}
          allTags={allTags}
          saving={modalSaving}
          error={modalError}
          onSave={handleSaveModal}
          onClose={() => setModal(null)}
        />
      )}
    </>
  )
}

interface RecordModalProps {
  title: string
  initial: Pick<KnowledgeRecord, 'title' | 'content' | 'tags'>
  allTags: string[]
  saving: boolean
  error: string | null
  onSave: (record: { title: string; content: string; tags?: string[] }) => void
  onClose: () => void
}

function RecordModal({ title, initial, allTags, saving, error, onSave, onClose }: RecordModalProps) {
  const [recTitle, setRecTitle] = useState(initial.title)
  const [content, setContent] = useState(initial.content)
  const [tags, setTags] = useState<string[]>(initial.tags ?? [])

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!recTitle.trim() || !content.trim()) return
    onSave({ title: recTitle.trim(), content: content.trim(), tags: tags.length > 0 ? tags : undefined })
  }

  return (
    <Modal title={title} onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Title
          <input type="text" value={recTitle} onChange={(e) => setRecTitle(e.target.value)} autoFocus />
        </label>
        <label>
          Content
          <textarea rows={6} value={content} onChange={(e) => setContent(e.target.value)} />
        </label>
        <label>
          Tags
          <TagInput tags={tags} onChange={setTags} suggestions={allTags} />
        </label>
        {error && <p className="error">{error}</p>}
        <ModalActions onCancel={onClose} busy={saving} disabled={!recTitle.trim() || !content.trim()} />
      </form>
    </Modal>
  )
}

export default KnowledgeDetail
