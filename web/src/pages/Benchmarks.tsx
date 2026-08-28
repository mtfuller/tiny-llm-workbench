import { Plus, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { deleteBenchmark, listBenchmarks, saveBenchmark, type Benchmark } from '../api'
import IconButton from '../IconButton'
import ListPanel from '../ListPanel'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import { useResourceList } from '../useResourceList'

function Benchmarks() {
  const navigate = useNavigate()
  const list = useResourceList<Benchmark>({
    load: listBenchmarks,
    getName: (b) => b.name,
    remove: (b) => deleteBenchmark(b.name),
    confirmMessage: (b) => `Delete benchmark "${b.name}"? This cannot be undone.`,
    deletedToast: (b) => `Deleted benchmark "${b.name}"`,
  })

  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const handleCreate = async (name: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await saveBenchmark({ name, testCases: [] })
      setCreateOpen(false)
      // A brand-new benchmark has no test cases yet — send the user straight
      // to its detail page to add some.
      navigate(`/benchmarks/${encodeURIComponent(name)}`)
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Benchmarks</h2>
      </div>
      <p className="hint">
        Define test cases (a prompt plus assertions on the reply) and run them against a set of models
        to compare how they perform.
      </p>

      <ListPanel
        search={list.search}
        onSearch={list.setSearch}
        searchPlaceholder="Search benchmarks…"
        actions={<IconButton icon={<Plus size={16} />} label="New benchmark" onClick={() => setCreateOpen(true)} />}
        error={list.error}
        loading={list.items === null}
        isEmpty={list.items !== null && list.items.length === 0}
        hasMatches={list.filtered.length > 0}
        emptyMessage="No benchmarks yet. Create one above."
        noMatchMessage="No benchmarks match your search."
        skeletonColumns={4}
        page={list.page}
        pageCount={list.pageCount}
        setPage={list.setPage}
        shownCount={list.filtered.length}
        totalCount={list.items?.length ?? 0}
        itemLabel="benchmarks"
      >
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Version</th>
              <th>Test cases</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.pageItems.map((b) => (
              <tr key={b.name}>
                <td>
                  <Link to={`/benchmarks/${encodeURIComponent(b.name)}`}>{b.name}</Link>
                </td>
                <td>v{b.version}</td>
                <td>{b.testCases.length}</td>
                <td className="row-actions">
                  <IconButton
                    icon={<Trash2 size={15} />}
                    label="Delete benchmark"
                    disabled={list.deleting === b.name}
                    onClick={() => list.handleDelete(b)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ListPanel>

      {createOpen && (
        <CreateBenchmarkModal creating={creating} error={createError} onCreate={handleCreate} onClose={() => setCreateOpen(false)} />
      )}
    </>
  )
}

interface CreateBenchmarkModalProps {
  creating: boolean
  error: string | null
  onCreate: (name: string) => void
  onClose: () => void
}

function CreateBenchmarkModal({ creating, error, onCreate, onClose }: CreateBenchmarkModalProps) {
  const [name, setName] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    onCreate(name.trim())
  }

  return (
    <Modal title="New benchmark" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </label>
        <p className="hint">You'll add test cases on the benchmark's own page next.</p>

        {error && <p className="error">{error}</p>}
        <ModalActions onCancel={onClose} submitLabel="Create" busyLabel="Creating…" busy={creating} disabled={!name.trim()} />
      </form>
    </Modal>
  )
}

export default Benchmarks
