import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { createDataset, deleteDataset, listDatasets, type DatasetSummary } from '../api'

function Datasets() {
  const [datasets, setDatasets] = useState<DatasetSummary[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [newName, setNewName] = useState('')
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<string | null>(null)

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

  const handleDelete = async (name: string) => {
    if (!window.confirm(`Delete dataset "${name}"? This cannot be undone.`)) return

    setDeleting(name)
    setError(null)
    try {
      await deleteDataset(name)
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
                <th></th>
              </tr>
            </thead>
            <tbody>
              {datasets.map((dataset) => (
                <tr key={dataset.name}>
                  <td>
                    <Link to={`/datasets/${encodeURIComponent(dataset.name)}`}>{dataset.name}</Link>
                  </td>
                  <td>{dataset.pairCount}</td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="danger-button"
                      disabled={deleting === dataset.name}
                      onClick={() => handleDelete(dataset.name)}
                    >
                      {deleting === dataset.name ? 'Deleting…' : 'Delete'}
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

export default Datasets
