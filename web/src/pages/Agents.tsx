import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { listAgents, saveAgent, type Agent } from '../api'

function Agents() {
  const navigate = useNavigate()
  const [agents, setAgents] = useState<Agent[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [newName, setNewName] = useState('')
  const [creating, setCreating] = useState(false)

  useEffect(() => {
    listAgents()
      .then(setAgents)
      .catch((err: Error) => setError(err.message))
  }, [])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newName.trim()) return

    setCreating(true)
    setError(null)
    try {
      const startX = 60
      await saveAgent(newName.trim(), {
        nodes: [
          { id: 'input-1', type: 'input', position: { x: startX, y: 120 }, data: { label: 'Input' } },
          { id: 'output-1', type: 'output', position: { x: startX + 500, y: 120 }, data: { label: 'Output' } },
        ],
        edges: [],
      })
      navigate(`/agents/${encodeURIComponent(newName.trim())}`)
    } catch (err) {
      setError((err as Error).message)
      setCreating(false)
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>Agents</h2>
      </div>
      <p className="hint">Design agent workflows on a canvas, then chat with them to try them out.</p>

      <form className="inline-form" onSubmit={handleCreate}>
        <input type="text" placeholder="New agent name" value={newName} onChange={(e) => setNewName(e.target.value)} />
        <button type="submit" disabled={creating || !newName.trim()}>
          {creating ? 'Creating…' : 'New agent'}
        </button>
      </form>

      {error && <p className="error">{error}</p>}

      {!error && agents === null && <p className="hint">Loading…</p>}

      {agents !== null && agents.length === 0 && (
        <p className="empty-state">No agents yet. Create one above to open the canvas.</p>
      )}

      {agents !== null && agents.length > 0 && (
        <div className="panel panel-flush">
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Nodes</th>
              </tr>
            </thead>
            <tbody>
              {agents.map((agent) => (
                <tr key={agent.name}>
                  <td>
                    <Link to={`/agents/${encodeURIComponent(agent.name)}`}>{agent.name}</Link>
                  </td>
                  <td>{agent.graph.nodes.length}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}

export default Agents
