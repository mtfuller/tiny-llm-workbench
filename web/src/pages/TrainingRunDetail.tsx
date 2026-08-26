import { AlertTriangle } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { cancelTrainingRun, getTrainingRun, type TrainingRun } from '../api'
import { useConfirm } from '../ConfirmDialog'
import { useEventStream } from '../eventStream'
import LinkArrow from '../LinkArrow'
import LossChart from '../LossChart'
import RunStats from '../RunStats'
import Skeleton from '../Skeleton'
import { useToast } from '../Toast'

function statusClass(status: TrainingRun['status']): string {
  if (status === 'succeeded') return 'status-open'
  if (status === 'failed' || status === 'cancelled') return 'status-closed'
  return 'status-connecting'
}

function TrainingRunDetail() {
  const { id = '' } = useParams<{ id: string }>()
  const confirm = useConfirm()
  const showToast = useToast()
  const { subscribe } = useEventStream()
  const [run, setRun] = useState<TrainingRun | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [cancelling, setCancelling] = useState(false)

  useEffect(() => {
    getTrainingRun(id)
      .then(setRun)
      .catch((err: Error) => setError(err.message))
  }, [id])

  useEffect(() => {
    const unsubscribeStatus = subscribe('training.status', (event) => {
      const updated = JSON.parse(event.data) as TrainingRun
      if (updated.id === id) setRun(updated)
    })
    const unsubscribeProgress = subscribe('training.progress', (event) => {
      const point = JSON.parse(event.data) as TrainingRun['progress'][number] & { runId: string }
      if (point.runId !== id) return
      setRun((prev) => (prev ? { ...prev, progress: [...prev.progress, point] } : prev))
    })
    return () => {
      unsubscribeStatus()
      unsubscribeProgress()
    }
  }, [subscribe, id])

  // Same SSE-race lesson as the Training list page: poll while this run is
  // still active so the page reconciles even if a fast-finishing run's
  // status event fires before the EventSource subscription is ready.
  useEffect(() => {
    if (run?.status !== 'running') return
    const interval = setInterval(() => {
      getTrainingRun(id)
        .then(setRun)
        .catch(() => {})
    }, 3000)
    return () => clearInterval(interval)
  }, [run?.status, id])

  const handleCancel = async () => {
    if (!run || !(await confirm(`Cancel training run "${run.config.outputName}"? This stops it permanently.`))) return

    setCancelling(true)
    setError(null)
    try {
      await cancelTrainingRun(run.id)
      showToast('Training run cancelled')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setCancelling(false)
    }
  }

  if (error) {
    return (
      <>
        <div className="page-header">
          <h2>
            <Link to="/training">Training</Link> / {id}
          </h2>
        </div>
        <p className="error">{error}</p>
      </>
    )
  }

  if (!run) {
    return (
      <>
        <div className="page-header">
          <h2>
            <Link to="/training">Training</Link> / {id}
          </h2>
        </div>
        <section className="panel">
          <div className="stat-row">
            {Array.from({ length: 5 }).map((_, i) => (
              <div className="stat-tile" key={i}>
                <Skeleton height="1.3rem" width="70%" />
                <div style={{ marginTop: '0.35rem' }}>
                  <Skeleton height="0.7rem" width="50%" />
                </div>
              </div>
            ))}
          </div>
        </section>
        <section className="panel">
          <Skeleton height="200px" />
        </section>
      </>
    )
  }

  return (
    <>
      <div className="page-header">
        <h2>
          <Link to="/training">Training</Link> / {run.config.outputName}
        </h2>
        <div className="row-actions">
          <span className={`status ${statusClass(run.status)}`}>{run.status}</span>
          {run.status === 'running' && (
            <button type="button" className="danger-button" disabled={cancelling} onClick={handleCancel}>
              {cancelling ? 'Cancelling…' : 'Cancel'}
            </button>
          )}
        </div>
      </div>

      {run.status === 'failed' && run.error && (
        <div className="alert-banner">
          <AlertTriangle size={16} />
          <span>{run.error}</span>
        </div>
      )}

      {run.status === 'succeeded' && (
        <p className="hint">
          Registered as model <code>{run.config.outputName}</code> —{' '}
          <LinkArrow to={`/models/${encodeURIComponent(run.config.outputName)}`}>view model</LinkArrow>
        </p>
      )}

      <section className="panel">
        <RunStats run={run} />
      </section>

      <section className="panel">
        <h3>Loss</h3>
        <LossChart progress={run.progress} />
      </section>

      <section className="panel">
        <h3>Config</h3>
        <dl className="info-list">
          <dt>Base model</dt>
          <dd>
            <code>{run.config.baseModel}</code>
          </dd>
          <dt>Dataset</dt>
          <dd>{run.config.dataset}</dd>
          <dt>Iterations</dt>
          <dd>{run.config.iterations}</dd>
          <dt>Learning rate</dt>
          <dd>{run.config.learningRate ?? 'default'}</dd>
          <dt>Started</dt>
          <dd>{new Date(run.startedAt).toLocaleString()}</dd>
          <dt>Finished</dt>
          <dd>{run.finishedAt ? new Date(run.finishedAt).toLocaleString() : '—'}</dd>
        </dl>
      </section>
    </>
  )
}

export default TrainingRunDetail
