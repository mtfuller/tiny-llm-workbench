import { Copy } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { getWorkspace, type Workspace } from '../api'
import { Badge } from '../Badge'
import { formatDateTime } from '../lib/format'
import { useToast } from '../Toast'

function WorkspaceDetail() {
  const { name = '' } = useParams<{ name: string }>()
  const showToast = useToast()
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [error, setError] = useState<string | null>(null)

  const reload = useCallback(() => {
    getWorkspace(name)
      .then(setWorkspace)
      .catch((err: Error) => setError(err.message))
  }, [name])

  useEffect(() => {
    reload()
  }, [reload])

  const copyPath = () => {
    if (!workspace) return
    navigator.clipboard?.writeText(workspace.hostPath).then(
      () => showToast('Path copied'),
      () => showToast('Could not copy'),
    )
  }

  return (
    <>
      <div className="page-header">
        <h2>
          <Link to="/workspaces">Workspaces</Link> / {name}
        </h2>
      </div>

      {error && <p className="error">{error}</p>}

      {workspace && (
        <div className="panel">
          <h3>
            Workspace info <Badge>{workspace.type}</Badge>
          </h3>
          <dl className="info-list">
            <dt>Type</dt>
            <dd>{workspace.type === 'test' ? 'Test — copied into a fresh sandbox per run' : 'Real — bind-mounted, changes persist'}</dd>
            <dt>Location</dt>
            <dd>
              <code>{workspace.hostPath}</code>{' '}
              <button type="button" className="icon-button" title="Copy path" aria-label="Copy path" onClick={copyPath}>
                <Copy size={14} />
              </button>
            </dd>
            <dt>Created</dt>
            <dd>{formatDateTime(workspace.createdAt)}</dd>
          </dl>

          {workspace.type === 'test' ? (
            <p className="hint">
              Open <code>{workspace.hostPath}</code> in your editor to set up the starting files an agent
              or evaluation will see. Every run gets its own throwaway copy — nothing an agent changes
              flows back here.
            </p>
          ) : (
            <p className="hint">
              An agent bound to this workspace (via a Deployment) acts directly on{' '}
              <code>{workspace.hostPath}</code>. Its changes persist.
            </p>
          )}
        </div>
      )}
    </>
  )
}

export default WorkspaceDetail
