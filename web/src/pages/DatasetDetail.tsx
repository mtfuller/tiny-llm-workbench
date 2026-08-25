import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { generateVariations, getDataset, listModels, type DatasetDetail as DatasetDetailType, type Model } from '../api'

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

      {error && <p className="error">{error}</p>}

      {!error && dataset === null && <p className="hint">Loading…</p>}

      {dataset !== null && dataset.examples.length === 0 && (
        <p className="empty-state">No examples yet. Generate some above.</p>
      )}

      {dataset !== null && dataset.examples.length > 0 && (
        <div className="panel panel-flush">
          <table className="data-table">
            <thead>
              <tr>
                <th>Input</th>
                <th>Output</th>
              </tr>
            </thead>
            <tbody>
              {dataset.examples.map((example, i) => (
                <tr key={i}>
                  <td>{example.input}</td>
                  <td>{example.output}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}

export default DatasetDetail
