import { Plus, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { deleteEvaluation, listEvaluations, saveEvaluation, type Evaluation } from '../api'
import IconButton from '../IconButton'
import ListPanel from '../ListPanel'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import { useResourceList } from '../useResourceList'

function Evaluations() {
  const navigate = useNavigate()
  const list = useResourceList<Evaluation>({
    load: listEvaluations,
    getName: (e) => e.name,
    remove: (e) => deleteEvaluation(e.name),
    confirmMessage: (e) => `Delete evaluation "${e.name}"? This cannot be undone.`,
    deletedToast: (e) => `Deleted evaluation "${e.name}"`,
  })

  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const handleCreate = async (name: string) => {
    setCreating(true)
    setCreateError(null)
    try {
      await saveEvaluation({ name })
      setCreateOpen(false)
      // A brand-new evaluation has no test cases yet — send the user
      // straight to its detail page to add some.
      navigate(`/evaluations/${encodeURIComponent(name)}`)
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Evaluations</h2>
      </div>
      <p className="hint">
        Define test cases — a prompt, an optional test workspace to seed a starting scenario, and
        assertions on the reply and the sandbox's resulting state — and run them against a set of agents
        to see how well they actually complete real software-dev, knowledge-work, or office-work tasks.
      </p>

      <ListPanel
        search={list.search}
        onSearch={list.setSearch}
        searchPlaceholder="Search evaluations…"
        actions={<IconButton icon={<Plus size={16} />} label="New evaluation" onClick={() => setCreateOpen(true)} />}
        error={list.error}
        loading={list.items === null}
        isEmpty={list.items !== null && list.items.length === 0}
        hasMatches={list.filtered.length > 0}
        emptyMessage="No evaluations yet. Create one above."
        noMatchMessage="No evaluations match your search."
        skeletonColumns={4}
        page={list.page}
        pageCount={list.pageCount}
        setPage={list.setPage}
        shownCount={list.filtered.length}
        totalCount={list.items?.length ?? 0}
        itemLabel="evaluations"
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
            {list.pageItems.map((evaluation) => (
              <tr key={evaluation.name}>
                <td>
                  <Link to={`/evaluations/${encodeURIComponent(evaluation.name)}`}>{evaluation.name}</Link>
                </td>
                <td>v{evaluation.version}</td>
                <td>{evaluation.testCases.length}</td>
                <td className="row-actions">
                  <IconButton
                    icon={<Trash2 size={15} />}
                    label="Delete evaluation"
                    disabled={list.deleting === evaluation.name}
                    onClick={() => list.handleDelete(evaluation)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ListPanel>

      {createOpen && (
        <CreateEvaluationModal
          creating={creating}
          error={createError}
          onCreate={handleCreate}
          onClose={() => setCreateOpen(false)}
        />
      )}
    </>
  )
}

interface CreateEvaluationModalProps {
  creating: boolean
  error: string | null
  onCreate: (name: string) => void
  onClose: () => void
}

function CreateEvaluationModal({ creating, error, onCreate, onClose }: CreateEvaluationModalProps) {
  const [name, setName] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    onCreate(name.trim())
  }

  return (
    <Modal title="New evaluation" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Name
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </label>
        <p className="hint">You'll add test cases on the evaluation's own page next.</p>

        {error && <p className="error">{error}</p>}
        <ModalActions onCancel={onClose} submitLabel="Create" busyLabel="Creating…" busy={creating} disabled={!name.trim()} />
      </form>
    </Modal>
  )
}

export default Evaluations
