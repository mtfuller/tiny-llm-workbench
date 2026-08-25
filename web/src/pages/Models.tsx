import { useEffect, useState } from 'react'
import { listModels, type Model } from '../api'

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

  useEffect(() => {
    listModels()
      .then(setModels)
      .catch((err: Error) => setError(err.message))
  }, [])

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
