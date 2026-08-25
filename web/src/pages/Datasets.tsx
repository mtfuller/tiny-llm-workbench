import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { createDataset, listDatasets, type DatasetSummary } from '../api'

function Datasets() {
  const [datasets, setDatasets] = useState<DatasetSummary[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [newName, setNewName] = useState('')
  const [creating, setCreating] = useState(false)

  const reload = () => {
    listDatasets()
      .then(setDatasets)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(reload, [])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newName.trim()) return

    setCreating(true)
    setError(null)
    try {
      await createDataset(newName.trim())
      setNewName('')
      reload()
    } catch (err) {
      setError((err as Error).message)
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

      <form className="inline-form" onSubmit={handleCreate}>
        <input
          type="text"
          placeholder="New dataset name"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
        />
        <button type="submit" disabled={creating || !newName.trim()}>
          {creating ? 'Creating…' : 'Create dataset'}
        </button>
      </form>

      {error && <p className="error">{error}</p>}

      {!error && datasets === null && <p className="hint">Loading…</p>}

      {datasets !== null && datasets.length === 0 && (
        <p className="empty-state">No datasets yet. Create one above to get started.</p>
      )}

      {datasets !== null && datasets.length > 0 && (
        <div className="panel panel-flush">
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Pairs</th>
              </tr>
            </thead>
            <tbody>
              {datasets.map((dataset) => (
                <tr key={dataset.name}>
                  <td>
                    <Link to={`/datasets/${encodeURIComponent(dataset.name)}`}>{dataset.name}</Link>
                  </td>
                  <td>{dataset.pairCount}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}

export default Datasets
