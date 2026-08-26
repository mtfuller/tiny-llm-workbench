import { useEffect, useState } from 'react'
import { deleteModel, listModels, type Model } from '../api'

function formatSize(bytes?: number): string {
  if (!bytes) return '—'
  const gb = bytes / 1_000_000_000
  if (gb >= 1) return `${gb.toFixed(1)} GB`
  return `${(bytes / 1_000_000).toFixed(0)} MB`
}

function sourceBadgeClass(source: string): string {
  if (source === 'ollama') return 'badge badge-blue'
  if (source === 'mlx') return 'badge badge-purple'
  return 'badge'
}

function Models() {
  const [models, setModels] = useState<Model[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<string | null>(null)

  const reload = () => {
    listModels()
      .then(setModels)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(reload, [])

  const handleDelete = async (model: Model) => {
    if (!window.confirm(`Delete model "${model.name}"? This cannot be undone.`)) return

    setDeleting(model.name)
    setError(null)
    try {
      await deleteModel(model.name, model.source)
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
        Local models pulled with Ollama, plus anything trained or imported into TLW's own registry.
      </p>

      {error && <p className="error">Failed to load models: {error}</p>}

      {!error && models === null && <p className="hint">Loading…</p>}

      {models !== null && models.length === 0 && (
        <p className="empty-state">
          No models found. Pull one with Ollama (<code>ollama pull llama3.2</code>) or train one in TLW.
        </p>
      )}

      {models !== null && models.length > 0 && (
        <div className="panel panel-flush">
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Source</th>
                <th>Size</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {models.map((model) => (
                <tr key={model.name}>
                  <td>{model.name}</td>
                  <td>
                    <span className={sourceBadgeClass(model.source)}>{model.source}</span>
                  </td>
                  <td>{formatSize(model.size)}</td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="danger-button"
                      disabled={deleting === model.name}
                      onClick={() => handleDelete(model)}
                    >
                      {deleting === model.name ? 'Deleting…' : 'Delete'}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}

export default Models
