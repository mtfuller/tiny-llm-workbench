import { useEffect, useMemo, useState } from 'react'
import {
  listDatasets,
  listTrainingRuns,
  startTrainingRun,
  type DatasetSummary,
  type ProgressPoint,
  type TrainingRun,
} from '../api'
import { useEventStream } from '../eventStream'

function formatDuration(startedAt: string, finishedAt?: string): string {
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now()
  const seconds = Math.max(0, Math.round((end - new Date(startedAt).getTime()) / 1000))
  if (seconds < 60) return `${seconds}s`
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}

function latestPoint(run: TrainingRun): ProgressPoint | undefined {
  return run.progress[run.progress.length - 1]
}

function statusClass(status: TrainingRun['status']): string {
  if (status === 'succeeded') return 'status-open'
  if (status === 'failed') return 'status-closed'
  return 'status-connecting'
}

function Training() {
  const { subscribe } = useEventStream()
  const [datasets, setDatasets] = useState<DatasetSummary[]>([])
  const [runs, setRuns] = useState<TrainingRun[]>([])
  const [error, setError] = useState<string | null>(null)

  const [baseModel, setBaseModel] = useState('')
  const [dataset, setDataset] = useState('')
  const [outputName, setOutputName] = useState('')
  const [iterations, setIterations] = useState(200)
  const [learningRate, setLearningRate] = useState('')
  const [starting, setStarting] = useState(false)

  useEffect(() => {
    listDatasets()
      .then((list) => {
        setDatasets(list)
        if (list.length > 0) setDataset((current) => current || list[0].name)
      })
      .catch(() => setDatasets([]))
    listTrainingRuns()
      .then(setRuns)
      .catch((err: Error) => setError(err.message))
  }, [])

  useEffect(() => {
    const unsubscribeStatus = subscribe('training.status', (event) => {
      const run = JSON.parse(event.data) as TrainingRun
      setRuns((prev) => {
        const others = prev.filter((r) => r.id !== run.id)
        return [run, ...others]
      })
    })

    const unsubscribeProgress = subscribe('training.progress', (event) => {
      const point = JSON.parse(event.data) as ProgressPoint & { runId: string }
      setRuns((prev) =>
        prev.map((r) => (r.id === point.runId ? { ...r, progress: [...r.progress, point] } : r)),
      )
    })

    return () => {
      unsubscribeStatus()
      unsubscribeProgress()
    }
  }, [subscribe])

  const activeRun = useMemo(() => runs.find((r) => r.status === 'running'), [runs])

  // SSE delivers live updates, but a run that finishes almost immediately
  // (e.g. the training script fails to even launch) can have its
  // "training.status" event fire before this page's EventSource finishes
  // subscribing, leaving the UI stuck showing "running" forever. Poll while
  // a run is active so the UI is guaranteed to reconcile even if that
  // happens.
  useEffect(() => {
    if (!activeRun) return
    const interval = setInterval(() => {
      listTrainingRuns()
        .then(setRuns)
        .catch(() => {})
    }, 3000)
    return () => clearInterval(interval)
  }, [activeRun])

  const handleStart = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!baseModel.trim() || !dataset || !outputName.trim()) return

    setStarting(true)
    setError(null)
    try {
      const run = await startTrainingRun({
        baseModel: baseModel.trim(),
        dataset,
        outputName: outputName.trim(),
        iterations,
        learningRate: learningRate ? Number(learningRate) : undefined,
      })
      setRuns((prev) => [run, ...prev])
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setStarting(false)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Training</h2>
      </div>

      <section className="panel">
        <h3>Start a run</h3>
        <p className="hint">
          Fine-tunes a base model with LoRA via MLX. The base model must be an MLX-format model (a
          Hugging Face repo id such as <code>mlx-community/Qwen2.5-0.5B-Instruct-4bit</code>, or a
          model already trained in TLW) — Ollama models aren't MLX-compatible and can't be used here.
        </p>
        <form className="stacked-form" onSubmit={handleStart}>
          <label>
            Base model
            <input
              type="text"
              placeholder="mlx-community/Qwen2.5-0.5B-Instruct-4bit"
              value={baseModel}
              onChange={(e) => setBaseModel(e.target.value)}
            />
          </label>
          <label>
            Dataset
            <select value={dataset} onChange={(e) => setDataset(e.target.value)}>
              {datasets.length === 0 && <option value="">No datasets available</option>}
              {datasets.map((d) => (
                <option key={d.name} value={d.name}>
                  {d.name} ({d.pairCount} pairs)
                </option>
              ))}
            </select>
          </label>
          <label>
            Output model name
            <input type="text" value={outputName} onChange={(e) => setOutputName(e.target.value)} />
          </label>
          <label>
            Iterations
            <input
              type="number"
              min={1}
              value={iterations}
              onChange={(e) => setIterations(Number(e.target.value))}
            />
          </label>
          <label>
            Learning rate (optional)
            <input
              type="text"
              placeholder="default"
              value={learningRate}
              onChange={(e) => setLearningRate(e.target.value)}
            />
          </label>
          <button type="submit" disabled={starting || !!activeRun || !dataset}>
            {activeRun ? 'A run is already in progress' : starting ? 'Starting…' : 'Start training'}
          </button>
        </form>
      </section>

      {error && <p className="error">{error}</p>}

      <div className="page-header">
        <h3>Runs</h3>
      </div>

      {runs.length === 0 && <p className="empty-state">No training runs yet.</p>}

      {runs.length > 0 && (
        <div className="panel panel-flush">
        <table className="data-table">
          <thead>
            <tr>
              <th>Output</th>
              <th>Base model</th>
              <th>Dataset</th>
              <th>Status</th>
              <th>Iteration</th>
              <th>Train loss</th>
              <th>Peak mem</th>
              <th>Duration</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((run) => {
              const latest = latestPoint(run)
              return (
                <tr key={run.id}>
                  <td>{run.config.outputName}</td>
                  <td>{run.config.baseModel}</td>
                  <td>{run.config.dataset}</td>
                  <td>
                    <span className={`status ${statusClass(run.status)}`}>{run.status}</span>
                    {run.status === 'failed' && run.error && (
                      <div className="error">{run.error}</div>
                    )}
                  </td>
                  <td>
                    {latest ? `${latest.iteration} / ${run.config.iterations}` : '—'}
                  </td>
                  <td>{latest?.trainLoss !== undefined ? latest.trainLoss.toFixed(3) : '—'}</td>
                  <td>{latest?.peakMemGB !== undefined ? `${latest.peakMemGB.toFixed(1)} GB` : '—'}</td>
                  <td>{formatDuration(run.startedAt, run.finishedAt)}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
        </div>
      )}
    </>
  )
}

export default Training
