import { BarChart3, MessageSquare } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { getModel, type ModelDetail as ModelDetailType, type TrainingRun } from '../api'
import LinkArrow from '../LinkArrow'
import ModelArchitecture from '../ModelArchitecture'
import ModelChatModal from '../ModelChatModal'
import RunStats from '../RunStats'
import Skeleton from '../Skeleton'
import TokenProbabilitiesModal from '../TokenProbabilitiesModal'

function statusClass(status: TrainingRun['status']): string {
  if (status === 'succeeded') return 'status-open'
  if (status === 'failed' || status === 'cancelled') return 'status-closed'
  return 'status-connecting'
}

function ModelDetail() {
  const { name = '' } = useParams<{ name: string }>()
  const [model, setModel] = useState<ModelDetailType | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [chatOpen, setChatOpen] = useState(false)
  const [tokenProbOpen, setTokenProbOpen] = useState(false)

  useEffect(() => {
    getModel(name)
      .then(setModel)
      .catch((err: Error) => setError(err.message))
  }, [name])

  if (error) {
    return (
      <>
        <div className="page-header">
          <h2>
            <Link to="/models">Models</Link> / {name}
          </h2>
        </div>
        <p className="error">{error}</p>
      </>
    )
  }

  if (!model) {
    return (
      <>
        <div className="page-header">
          <h2>
            <Link to="/models">Models</Link> / {name}
          </h2>
        </div>
        <section className="panel">
          <Skeleton height="1.3rem" width="60%" />
        </section>
      </>
    )
  }

  return (
    <>
      <div className="page-header">
        <h2>
          <Link to="/models">Models</Link> / {model.name}
        </h2>
        <div className="row-actions">
          <button type="button" className="button-primary" onClick={() => setTokenProbOpen(true)}>
            <BarChart3 size={16} />
            Token probabilities
          </button>
          <button type="button" className="button-primary" onClick={() => setChatOpen(true)}>
            <MessageSquare size={16} />
            Chat with model
          </button>
        </div>
      </div>

      <div className="model-detail-grid">
        <section className="panel">
          <h3>Model info</h3>
          <dl className="info-list">
            <dt>Source</dt>
            <dd>{model.source}</dd>
            <dt>Base model</dt>
            <dd>{model.baseModel ? <code>{model.baseModel}</code> : '—'}</dd>
          </dl>
        </section>

        {model.trainingRun && (
          <section className="panel">
            <div className="page-header">
              <h3>Training run</h3>
              <span className={`status ${statusClass(model.trainingRun.status)}`}>{model.trainingRun.status}</span>
            </div>
            <RunStats run={model.trainingRun} />
            <p className="hint">
              <LinkArrow to={`/training/${model.trainingRun.id}`}>View full training details</LinkArrow>
            </p>
          </section>
        )}

        <section className="panel model-detail-card-wide">
          <h3>Architecture</h3>
          <ModelArchitecture modelName={model.name} />
        </section>
      </div>

      {chatOpen && <ModelChatModal modelName={model.name} onClose={() => setChatOpen(false)} />}
      {tokenProbOpen && <TokenProbabilitiesModal modelName={model.name} onClose={() => setTokenProbOpen(false)} />}
    </>
  )
}

export default ModelDetail
