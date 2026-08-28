import { Plus, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { createDataset, deleteDataset, listDatasets, type DatasetSummary } from '../api'
import IconButton from '../IconButton'
import ListPanel from '../ListPanel'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import { useToast } from '../Toast'
import { useResourceList } from '../useResourceList'

function Datasets() {
  const showToast = useToast()
  const list = useResourceList<DatasetSummary>({
    load: listDatasets,
    getName: (d) => d.name,
    remove: (d) => deleteDataset(d.name),
    confirmMessage: (d) => `Delete dataset "${d.name}"? This cannot be undone.`,
    deletedToast: (d) => `Deleted dataset "${d.name}"`,
  })

  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const handleCreate = async (name: string, title: string, description: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await createDataset(name, title || undefined, description || undefined)
      setCreateOpen(false)
      showToast(`Created dataset "${name}"`)
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
        <h2>Datasets</h2>
      </div>
      <p className="hint">Input/output training pairs used to fine-tune a model.</p>

      <ListPanel
        search={list.search}
        onSearch={list.setSearch}
        searchPlaceholder="Search datasets…"
        actions={<IconButton icon={<Plus size={16} />} label="New dataset" onClick={() => setCreateOpen(true)} />}
        error={list.error}
        loading={list.items === null}
        isEmpty={list.items !== null && list.items.length === 0}
        hasMatches={list.filtered.length > 0}
        emptyMessage="No datasets yet. Create one above to get started."
        noMatchMessage="No datasets match your search."
        skeletonColumns={3}
        page={list.page}
        pageCount={list.pageCount}
        setPage={list.setPage}
        shownCount={list.filtered.length}
        totalCount={list.items?.length ?? 0}
        itemLabel="datasets"
      >
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Pairs</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.pageItems.map((dataset) => (
              <tr key={dataset.name}>
                <td>
                  <Link to={`/datasets/${encodeURIComponent(dataset.name)}`}>{dataset.name}</Link>
                </td>
                <td>{dataset.pairCount}</td>
                <td className="row-actions">
                  <IconButton
                    icon={<Trash2 size={15} />}
                    label="Delete dataset"
                    disabled={list.deleting === dataset.name}
                    onClick={() => list.handleDelete(dataset)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ListPanel>

      {createOpen && (
        <CreateDatasetModal creating={creating} error={createError} onCreate={handleCreate} onClose={() => setCreateOpen(false)} />
      )}
    </>
  )
}

interface CreateDatasetModalProps {
  creating: boolean
  error: string | null
  onCreate: (name: string, title: string, description: string) => void
  onClose: () => void
}

function CreateDatasetModal({ creating, error, onCreate, onClose }: CreateDatasetModalProps) {
  const [name, setName] = useState('')
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    onCreate(name.trim(), title.trim(), description.trim())
  }

  return (
    <Modal title="New dataset" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </label>
        <label>
          Title (optional)
          <input type="text" placeholder="A short display name" value={title} onChange={(e) => setTitle(e.target.value)} />
        </label>
        <label>
          Description (optional)
          <textarea rows={3} value={description} onChange={(e) => setDescription(e.target.value)} />
        </label>
        {error && <p className="error">{error}</p>}
        <ModalActions onCancel={onClose} submitLabel="Create" busyLabel="Creating…" busy={creating} disabled={!name.trim()} />
      </form>
    </Modal>
  )
}

export default Datasets
