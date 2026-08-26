import { Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useConfirm } from '../ConfirmDialog'
import { deleteModel, listModels, type Model } from '../api'
import { TableSkeleton } from '../Skeleton'
import { useToast } from '../Toast'

function Models() {
  const confirm = useConfirm()
  const showToast = useToast()
  const [models, setModels] = useState<Model[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)

  const reload = () => {
    listModels()
      .then(setModels)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(reload, [])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return models ?? []
    return (models ?? []).filter((m) => m.name.toLowerCase().includes(q))
  }, [models, search])

  const handleDelete = async (model: Model) => {
    if (!(await confirm(`Delete model "${model.name}"? This cannot be undone.`))) return

    setDeleting(model.name)
    setError(null)
    try {
      await deleteModel(model.name)
      showToast(`Deleted model "${model.name}"`)
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
        <h2>Models</h2>
      </div>
      <p className="hint">
        Models trained in TLW. A Hugging Face MLX repo id (e.g.{' '}
        <code>mlx-community/Qwen2.5-0.5B-Instruct-4bit</code>) can also be used anywhere a model is
        picked, even before it appears here — it's downloaded automatically on first use.
      </p>

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search models…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
        </div>

        {error && (
          <div className="panel-body">
            <p className="error">Failed to load models: {error}</p>
          </div>
        )}

        {!error && models === null && (
          <div className="panel-body">
            <TableSkeleton columns={2} />
          </div>
        )}

        {models !== null && models.length === 0 && (
          <div className="panel-body">
            <p className="hint">No models yet. Train one on the Training page to get started.</p>
          </div>
        )}

        {models !== null && models.length > 0 && filtered.length === 0 && (
          <div className="panel-body">
            <p className="hint">No models match your search.</p>
          </div>
        )}

        {filtered.length > 0 && (
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((model) => (
                <tr key={model.name}>
                  <td>{model.name}</td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="icon-button"
                      title="Delete model"
                      aria-label="Delete model"
                      disabled={deleting === model.name}
                      onClick={() => handleDelete(model)}
                    >
                      <Trash2 size={15} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  )
}

export default Models
