import { useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  addExamples,
  datasetExportUrl,
  deleteExample,
  generateVariations,
  getDataset,
  importDataset,
  listModels,
  updateExample,
  type DatasetDetail as DatasetDetailType,
  type Example,
  type Model,
} from '../api'

const PAGE_SIZE = 20

function DatasetDetail() {
  const { name = '' } = useParams<{ name: string }>()
  const [dataset, setDataset] = useState<DatasetDetailType | null>(null)
  const [models, setModels] = useState<Model[]>([])
  const [error, setError] = useState<string | null>(null)

  const [selectedModel, setSelectedModel] = useState('')
  const [seedInput, setSeedInput] = useState('')
  const [seedOutput, setSeedOutput] = useState('')
  const [count, setCount] = useState(3)
  const [generating, setGenerating] = useState(false)

  const [newInput, setNewInput] = useState('')
  const [newOutput, setNewOutput] = useState('')
  const [adding, setAdding] = useState(false)

  const [importing, setImporting] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const [editingIndex, setEditingIndex] = useState<number | null>(null)
  const [editInput, setEditInput] = useState('')
  const [editOutput, setEditOutput] = useState('')
  const [saving, setSaving] = useState(false)
  const [deletingIndex, setDeletingIndex] = useState<number | null>(null)

  const [page, setPage] = useState(0)

  const reload = () => {
    getDataset(name)
      .then(setDataset)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(reload, [name])
  useEffect(() => {
    listModels()
      .then((list) => {
        setModels(list)
        if (list.length > 0) setSelectedModel((current) => current || list[0].name)
      })
      .catch(() => setModels([]))
  }, [])

  const handleGenerate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedModel || !seedInput.trim() || !seedOutput.trim()) return

    setGenerating(true)
    setError(null)
    try {
      await generateVariations(name, {
        model: selectedModel,
        seed: { input: seedInput.trim(), output: seedOutput.trim() },
        count,
      })
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setGenerating(false)
    }
  }

  const handleAddExample = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newInput.trim() || !newOutput.trim()) return

    setAdding(true)
    setError(null)
    try {
      await addExamples(name, [{ input: newInput.trim(), output: newOutput.trim() }])
      setNewInput('')
      setNewOutput('')
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setAdding(false)
    }
  }

  const handleImportFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

    const format = file.name.toLowerCase().endsWith('.csv') ? 'csv' : 'jsonl'
    setImporting(true)
    setError(null)
    try {
      const content = await file.text()
      await importDataset(name, format, content)
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setImporting(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  const startEdit = (index: number, example: Example) => {
    setEditingIndex(index)
    setEditInput(example.input)
    setEditOutput(example.output)
  }

  const cancelEdit = () => setEditingIndex(null)

  const handleSaveEdit = async (index: number) => {
    if (!editInput.trim() || !editOutput.trim()) return

    setSaving(true)
    setError(null)
    try {
      await updateExample(name, index, { input: editInput.trim(), output: editOutput.trim() })
      setEditingIndex(null)
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const handleDeleteExample = async (index: number) => {
    if (!window.confirm('Delete this example? This cannot be undone.')) return

    setDeletingIndex(index)
    setError(null)
    try {
      await deleteExample(name, index)
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setDeletingIndex(null)
    }
  }

  const examples = dataset?.examples ?? []
  const pageCount = Math.max(1, Math.ceil(examples.length / PAGE_SIZE))
  const currentPage = Math.min(page, pageCount - 1)
  const pageStart = currentPage * PAGE_SIZE
  const pageExamples = examples.slice(pageStart, pageStart + PAGE_SIZE)

  return (
    <>
      <div className="page-header">
        <h2>
          <Link to="/datasets">Datasets</Link> / {name}
        </h2>
      </div>

      <section className="panel">
        <h3>Generate variations</h3>
        <p className="hint">
          Give one example input/output pair and a local model will generate similar ones.
        </p>
        <form className="stacked-form" onSubmit={handleGenerate}>
          <label>
            Model
            <select value={selectedModel} onChange={(e) => setSelectedModel(e.target.value)}>
              {models.length === 0 && <option value="">No models available</option>}
              {models.map((model) => (
                <option key={model.name} value={model.name}>
                  {model.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Example input
            <input type="text" value={seedInput} onChange={(e) => setSeedInput(e.target.value)} />
          </label>
          <label>
            Example output
            <input type="text" value={seedOutput} onChange={(e) => setSeedOutput(e.target.value)} />
          </label>
          <label>
            How many variations
            <input
              type="number"
              min={1}
              max={20}
              value={count}
              onChange={(e) => setCount(Number(e.target.value))}
            />
          </label>
          <button type="submit" disabled={generating || !selectedModel}>
            {generating ? 'Generating…' : 'Generate'}
          </button>
        </form>
      </section>

      <section className="panel">
        <h3>Add an example</h3>
        <form className="inline-form" onSubmit={handleAddExample}>
          <input
            type="text"
            placeholder="Input"
            value={newInput}
            onChange={(e) => setNewInput(e.target.value)}
          />
          <input
            type="text"
            placeholder="Output"
            value={newOutput}
            onChange={(e) => setNewOutput(e.target.value)}
          />
          <button type="submit" disabled={adding || !newInput.trim() || !newOutput.trim()}>
            {adding ? 'Adding…' : 'Add example'}
          </button>
        </form>
      </section>

      <section className="panel">
        <h3>Import / export</h3>
        <div className="row-actions">
          <label className="button-file">
            {importing ? 'Importing…' : 'Import CSV/JSONL'}
            <input
              ref={fileInputRef}
              type="file"
              accept=".csv,.jsonl,.txt"
              onChange={handleImportFile}
              disabled={importing}
              hidden
            />
          </label>
          <a className="button-like" href={datasetExportUrl(name, 'jsonl')}>
            Export JSONL
          </a>
          <a className="button-like" href={datasetExportUrl(name, 'csv')}>
            Export CSV
          </a>
        </div>
        <p className="hint">
          A CSV import expects an <code>input,output</code> header row.
        </p>
      </section>

      {error && <p className="error">{error}</p>}

      {!error && dataset === null && <p className="hint">Loading…</p>}

      {dataset !== null && examples.length === 0 && (
        <p className="empty-state">No examples yet. Add one above or generate some.</p>
      )}

      {dataset !== null && examples.length > 0 && (
        <div className="panel panel-flush">
          <table className="data-table">
            <thead>
              <tr>
                <th>Input</th>
                <th>Output</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {pageExamples.map((example, i) => {
                const index = pageStart + i
                const isEditing = editingIndex === index
                return (
                  <tr key={index}>
                    {isEditing ? (
                      <>
                        <td>
                          <input type="text" value={editInput} onChange={(e) => setEditInput(e.target.value)} />
                        </td>
                        <td>
                          <input type="text" value={editOutput} onChange={(e) => setEditOutput(e.target.value)} />
                        </td>
                        <td className="row-actions">
                          <button type="button" disabled={saving} onClick={() => handleSaveEdit(index)}>
                            {saving ? 'Saving…' : 'Save'}
                          </button>
                          <button type="button" onClick={cancelEdit}>
                            Cancel
                          </button>
                        </td>
                      </>
                    ) : (
                      <>
                        <td>{example.input}</td>
                        <td>{example.output}</td>
                        <td className="row-actions">
                          <button type="button" onClick={() => startEdit(index, example)}>
                            Edit
                          </button>
                          <button
                            type="button"
                            className="danger-button"
                            disabled={deletingIndex === index}
                            onClick={() => handleDeleteExample(index)}
                          >
                            {deletingIndex === index ? 'Deleting…' : 'Delete'}
                          </button>
                        </td>
                      </>
                    )}
                  </tr>
                )
              })}
            </tbody>
          </table>
          {pageCount > 1 && (
            <div className="pagination">
              <button type="button" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>
                Previous
              </button>
              <span className="hint">
                Page {currentPage + 1} of {pageCount} ({examples.length} examples)
              </span>
              <button
                type="button"
                disabled={currentPage >= pageCount - 1}
                onClick={() => setPage(currentPage + 1)}
              >
                Next
              </button>
            </div>
          )}
        </div>
      )}
    </>
  )
}

export default DatasetDetail
