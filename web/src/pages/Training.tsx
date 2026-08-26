import { Ban, Plus } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import {
  cancelTrainingRun,
  listDatasets,
  listModels,
  listTrainingRuns,
  startTrainingRun,
  type DatasetSummary,
  type Model,
  type TrainingRun,
} from '../api'
import { useConfirm } from '../ConfirmDialog'
import { useEventStream } from '../eventStream'
import Modal from '../Modal'
import RunStats from '../RunStats'
import { TableSkeleton } from '../Skeleton'
import { suggestedModels } from '../suggestedModels'
import { useToast } from '../Toast'

function formatDuration(startedAt: string, finishedAt?: string): string {
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now()
  const seconds = Math.max(0, Math.round((end - new Date(startedAt).getTime()) / 1000))
  if (seconds < 60) return `${seconds}s`
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}

function statusClass(status: TrainingRun['status']): string {
  if (status === 'succeeded') return 'status-open'
  if (status === 'failed' || status === 'cancelled') return 'status-closed'
  return 'status-connecting'
}

function Training() {
  const navigate = useNavigate()
  const confirm = useConfirm()
  const showToast = useToast()
  const { subscribe } = useEventStream()
  const [datasets, setDatasets] = useState<DatasetSummary[]>([])
  const [models, setModels] = useState<Model[]>([])
  const [runs, setRuns] = useState<TrainingRun[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [cancelling, setCancelling] = useState<string | null>(null)

  const [createOpen, setCreateOpen] = useState(false)
  const [starting, setStarting] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  useEffect(() => {
    listDatasets()
      .then(setDatasets)
      .catch(() => setDatasets([]))
    listModels()
      .then(setModels)
      .catch(() => setModels([]))
    listTrainingRuns()
      .then(setRuns)
      .catch((err: Error) => setError(err.message))
  }, [])

  const baseModelOptions = useMemo(() => {
    const trained = models.map((m) => m.name)
    return Array.from(new Set([...trained, ...suggestedModels]))
  }, [models])

  useEffect(() => {
    const unsubscribeStatus = subscribe('training.status', (event) => {
      const run = JSON.parse(event.data) as TrainingRun
      setRuns((prev) => {
        const others = (prev ?? []).filter((r) => r.id !== run.id)
        return [run, ...others]
      })
    })

    const unsubscribeProgress = subscribe('training.progress', (event) => {
      const point = JSON.parse(event.data) as TrainingRun['progress'][number] & { runId: string }
      setRuns((prev) =>
        prev ? prev.map((r) => (r.id === point.runId ? { ...r, progress: [...r.progress, point] } : r)) : prev,
      )
    })

    return () => {
      unsubscribeStatus()
      unsubscribeProgress()
    }
  }, [subscribe])

  const activeRun = useMemo(() => runs?.find((r) => r.status === 'running'), [runs])

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

  const handleCancel = async (run: TrainingRun) => {
    if (!(await confirm(`Cancel training run "${run.config.outputName}"? This stops it permanently.`))) return

    setCancelling(run.id)
    setError(null)
    try {
      await cancelTrainingRun(run.id)
      showToast('Training run cancelled')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setCancelling(null)
    }
  }

  const handleStart = async (config: {
    baseModel: string
    dataset: string
    outputName: string
    iterations: number
    learningRate: string
  }) => {
    setStarting(true)
    setCreateError(null)
    try {
      const run = await startTrainingRun({
        baseModel: config.baseModel,
        dataset: config.dataset,
        outputName: config.outputName,
        iterations: config.iterations,
        learningRate: config.learningRate ? Number(config.learningRate) : undefined,
      })
      setRuns((prev) => [run, ...(prev ?? [])])
      navigate(`/training/${run.id}`)
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setStarting(false)
    }
  }

  const pastRuns = runs?.filter((r) => r.id !== activeRun?.id) ?? []

  const filteredRuns = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return pastRuns
    return pastRuns.filter(
      (r) =>
        r.config.outputName.toLowerCase().includes(q) ||
        r.config.baseModel.toLowerCase().includes(q) ||
        r.config.dataset.toLowerCase().includes(q) ||
        r.status.toLowerCase().includes(q),
    )
  }, [pastRuns, search])

  return (
    <>
      <div className="page-header">
        <h2>Training</h2>
      </div>

      {error && <p className="error">{error}</p>}

      {activeRun && (
        <section className="panel">
          <div className="page-header">
            <h3>
              Training <code>{activeRun.config.outputName}</code>
            </h3>
            <div className="row-actions">
              <span className={`status ${statusClass(activeRun.status)}`}>{activeRun.status}</span>
              <button
                type="button"
                className="danger-button"
                disabled={cancelling === activeRun.id}
                onClick={() => handleCancel(activeRun)}
              >
                {cancelling === activeRun.id ? 'Cancelling…' : 'Cancel'}
              </button>
            </div>
          </div>
          <RunStats run={activeRun} />
          <p className="hint">
            <Link to={`/training/${activeRun.id}`}>View live loss chart →</Link>
          </p>
        </section>
      )}

      <div className="page-header">
        <h3>Runs</h3>
      </div>

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search runs…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
          <div className="list-toolbar-actions">
            <button
              type="button"
              className="icon-button"
              title={activeRun ? 'A run is already in progress' : 'Start a run'}
              aria-label="Start a run"
              disabled={!!activeRun}
              onClick={() => setCreateOpen(true)}
            >
              <Plus size={16} />
            </button>
          </div>
        </div>

        {runs === null && (
          <div className="panel-body">
            <TableSkeleton columns={8} />
          </div>
        )}

        {runs !== null && runs.length === 0 && (
          <div className="panel-body">
            <p className="hint">No training runs yet.</p>
          </div>
        )}

        {pastRuns.length > 0 && filteredRuns.length === 0 && (
          <div className="panel-body">
            <p className="hint">No runs match your search.</p>
          </div>
        )}

        {filteredRuns.length > 0 && (
          <table className="data-table">
            <thead>
              <tr>
                <th>Output</th>
                <th>Base model</th>
                <th>Dataset</th>
                <th>Status</th>
                <th>Iteration</th>
                <th>Train loss</th>
                <th>Duration</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {filteredRuns.map((run) => {
                const latest = run.progress[run.progress.length - 1]
                return (
                  <tr key={run.id} className="run-row" onClick={() => navigate(`/training/${run.id}`)}>
                    <td>{run.config.outputName}</td>
                    <td>{run.config.baseModel}</td>
                    <td>{run.config.dataset}</td>
                    <td>
                      <span className={`status ${statusClass(run.status)}`}>{run.status}</span>
                      {run.status === 'failed' && run.error && <div className="error">{run.error}</div>}
                    </td>
                    <td>{latest ? `${latest.iteration} / ${run.config.iterations}` : '—'}</td>
                    <td>{latest?.trainLoss !== undefined ? latest.trainLoss.toFixed(3) : '—'}</td>
                    <td>{formatDuration(run.startedAt, run.finishedAt)}</td>
                    <td className="row-actions" onClick={(e) => e.stopPropagation()}>
                      {run.status === 'running' && (
                        <button
                          type="button"
                          className="icon-button"
                          title="Cancel run"
                          aria-label="Cancel run"
                          disabled={cancelling === run.id}
                          onClick={() => handleCancel(run)}
                        >
                          <Ban size={15} />
                        </button>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>

      {createOpen && (
        <StartRunModal
          datasets={datasets}
          baseModelOptions={baseModelOptions}
          starting={starting}
          error={createError}
          onStart={handleStart}
          onClose={() => setCreateOpen(false)}
        />
      )}
    </>
  )
}

interface StartRunModalProps {
  datasets: DatasetSummary[]
  baseModelOptions: string[]
  starting: boolean
  error: string | null
  onStart: (config: { baseModel: string; dataset: string; outputName: string; iterations: number; learningRate: string }) => void
  onClose: () => void
}

function StartRunModal({ datasets, baseModelOptions, starting, error, onStart, onClose }: StartRunModalProps) {
  const [baseModel, setBaseModel] = useState('')
  const [dataset, setDataset] = useState(datasets[0]?.name ?? '')
  const [outputName, setOutputName] = useState('')
  const [iterations, setIterations] = useState(200)
  const [learningRate, setLearningRate] = useState('')

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!baseModel.trim() || !dataset || !outputName.trim()) return
    onStart({ baseModel: baseModel.trim(), dataset, outputName: outputName.trim(), iterations, learningRate })
  }

  return (
    <Modal title="Start a run" onClose={onClose}>
      <p className="hint">
        Fine-tunes a base model with LoRA via MLX. The base model must be an MLX-format model — a
        Hugging Face repo id such as <code>mlx-community/Qwen2.5-0.5B-Instruct-4bit</code>, or a model
        already trained in TLW.
      </p>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Base model
          <input
            type="text"
            list="base-model-options"
            placeholder="mlx-community/Qwen2.5-0.5B-Instruct-4bit"
            value={baseModel}
            onChange={(e) => setBaseModel(e.target.value)}
            autoFocus
          />
          <datalist id="base-model-options">
            {baseModelOptions.map((name) => (
              <option key={name} value={name} />
            ))}
          </datalist>
        </label>
        <label>
          Output model name
          <input type="text" value={outputName} onChange={(e) => setOutputName(e.target.value)} />
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
          Iterations
          <input type="number" min={1} value={iterations} onChange={(e) => setIterations(Number(e.target.value))} />
        </label>
        <label>
          Learning rate (optional)
          <input type="text" placeholder="default" value={learningRate} onChange={(e) => setLearningRate(e.target.value)} />
        </label>
        {error && <p className="error">{error}</p>}
        <div className="row-actions confirm-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={starting || !dataset || !baseModel.trim() || !outputName.trim()}>
            {starting ? 'Starting…' : 'Start training'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

export default Training
