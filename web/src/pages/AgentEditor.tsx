import {
  addEdge,
  Background,
  Controls,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type Connection,
  type Edge,
  type Node,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { GitBranch, MessageSquare, PanelLeftClose, PanelLeftOpen, PanelRightClose, PanelRightOpen, Play, SquareArrowOutUpRight, Terminal } from 'lucide-react'
import { useCallback, useEffect, useRef, useState, type DragEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  getAgent,
  listEnvironments,
  listModels,
  saveAgent,
  sendAgentMessage,
  startAgentRun,
  stopAgentRun,
  type AgentNodeData,
  type AgentStepEvent,
  type ChatMessage,
  type Environment,
  type Model,
  type NodeType,
} from '../api'
import { nodeTypes } from '../agentNodes'
import { useEventStream } from '../eventStream'
import Modal from '../Modal'

type FlowNode = Node<AgentNodeData>

const PALETTE: { type: NodeType; label: string; icon: typeof MessageSquare }[] = [
  { type: 'prompt', label: 'Prompt', icon: MessageSquare },
  { type: 'decision', label: 'Decision', icon: GitBranch },
  { type: 'tool', label: 'Tool', icon: Terminal },
  { type: 'output', label: 'Output', icon: SquareArrowOutUpRight },
]

let nodeCounter = 0
function newNodeId(type: string): string {
  nodeCounter += 1
  return `${type}-${Date.now()}-${nodeCounter}`
}

function AgentEditor() {
  return (
    <ReactFlowProvider>
      <AgentEditorWorkspace />
    </ReactFlowProvider>
  )
}

function AgentEditorWorkspace() {
  const { name = '' } = useParams<{ name: string }>()
  const { subscribe } = useEventStream()
  const { screenToFlowPosition } = useReactFlow()
  const canvasRef = useRef<HTMLDivElement>(null)

  const [nodes, setNodes, onNodesChange] = useNodesState<FlowNode>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [models, setModels] = useState<Model[]>([])
  const [environments, setEnvironments] = useState<Environment[]>([])
  const [environment, setEnvironment] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [paletteOpen, setPaletteOpen] = useState(true)
  const [inspectorOpen, setInspectorOpen] = useState(true)

  const [chatOpen, setChatOpen] = useState(false)
  const [runId, setRunId] = useState<string | null>(null)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [chatInput, setChatInput] = useState('')
  const [sending, setSending] = useState(false)
  const [steps, setSteps] = useState<AgentStepEvent[]>([])
  const messagesEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    getAgent(name)
      .then((agent) => {
        setNodes(agent.graph.nodes as FlowNode[])
        setEdges(agent.graph.edges as Edge[])
        setEnvironment(agent.environment ?? '')
        setLoaded(true)
      })
      .catch((err: Error) => setError(err.message))
    listModels()
      .then(setModels)
      .catch(() => setModels([]))
    listEnvironments()
      .then(setEnvironments)
      .catch(() => setEnvironments([]))
  }, [name, setNodes, setEdges])

  useEffect(() => {
    return subscribe('agent.step', (event) => {
      const step = JSON.parse(event.data) as AgentStepEvent
      setSteps((prev) => (step.runId === runId ? [...prev, step] : prev))
    })
  }, [subscribe, runId])

  useEffect(() => {
    if (chatOpen) messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, chatOpen])

  const onConnect = useCallback(
    (connection: Connection) => setEdges((eds) => addEdge({ ...connection, id: `e-${Date.now()}` }, eds)),
    [setEdges],
  )

  const addNode = useCallback(
    (type: NodeType, position?: { x: number; y: number }) => {
      const id = newNodeId(type)
      const data: AgentNodeData = type === 'prompt' ? { model: models[0]?.name } : {}
      setNodes((nds) => [
        ...nds,
        { id, type, position: position ?? { x: 120 + nds.length * 40, y: 120 + (nds.length % 4) * 90 }, data },
      ])
    },
    [models, setNodes],
  )

  const onDragStart = (event: DragEvent, type: NodeType) => {
    event.dataTransfer.setData('application/tlw-node-type', type)
    event.dataTransfer.effectAllowed = 'move'
  }

  const onDragOver = useCallback((event: DragEvent) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }, [])

  const onDrop = useCallback(
    (event: DragEvent) => {
      event.preventDefault()
      const type = event.dataTransfer.getData('application/tlw-node-type') as NodeType
      if (!type) return
      const position = screenToFlowPosition({ x: event.clientX, y: event.clientY })
      addNode(type, position)
    },
    [screenToFlowPosition, addNode],
  )

  const deleteSelectedNode = () => {
    if (!selectedNodeId) return
    setNodes((nds) => nds.filter((n) => n.id !== selectedNodeId))
    setEdges((eds) => eds.filter((e) => e.source !== selectedNodeId && e.target !== selectedNodeId))
    setSelectedNodeId(null)
  }

  const updateSelectedNodeData = (patch: Partial<AgentNodeData>) => {
    setNodes((nds) => nds.map((n) => (n.id === selectedNodeId ? { ...n, data: { ...n.data, ...patch } } : n)))
  }

  const selectedNode = nodes.find((n) => n.id === selectedNodeId)

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    try {
      await saveAgent(
        name,
        {
          nodes: nodes.map((n) => ({ id: n.id, type: n.type as NodeType, position: n.position, data: n.data })),
          edges: edges.map((e) => ({ id: e.id, source: e.source, sourceHandle: e.sourceHandle ?? undefined, target: e.target })),
        },
        environment || undefined,
      )
      setSavedAt(Date.now())
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const openChat = () => {
    setChatOpen(true)
    if (!runId) void handleStartRun()
  }

  const closeChat = () => {
    setChatOpen(false)
    if (runId) void stopAgentRun(runId).catch(() => {})
    setRunId(null)
    setMessages([])
    setSteps([])
  }

  const handleStartRun = async () => {
    setError(null)
    try {
      const run = await startAgentRun(name)
      setRunId(run.id)
      setMessages([])
      setSteps([])
    } catch (err) {
      setError((err as Error).message)
    }
  }

  const handleSendMessage = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!runId || !chatInput.trim()) return

    const userMessage: ChatMessage = { role: 'user', content: chatInput.trim(), timestamp: new Date().toISOString() }
    setMessages((prev) => [...prev, userMessage])
    setChatInput('')
    setSending(true)
    setError(null)
    try {
      const reply = await sendAgentMessage(runId, userMessage.content)
      setMessages((prev) => [...prev, reply])
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="agent-editor">
      <div className="agent-editor-header">
        <h2>
          <Link to="/agents">Agents</Link> / {name}
        </h2>
        <div className="agent-editor-actions">
          <button
            type="button"
            className="icon-button"
            onClick={() => setPaletteOpen((o) => !o)}
            title={paletteOpen ? 'Hide node palette' : 'Show node palette'}
          >
            {paletteOpen ? <PanelLeftClose size={16} /> : <PanelLeftOpen size={16} />}
          </button>
          <button
            type="button"
            className="icon-button"
            onClick={() => setInspectorOpen((o) => !o)}
            title={inspectorOpen ? 'Hide inspector' : 'Show inspector'}
          >
            {inspectorOpen ? <PanelRightClose size={16} /> : <PanelRightOpen size={16} />}
          </button>
          <button type="button" onClick={handleSave} disabled={saving}>
            {saving ? 'Saving…' : 'Save'}
          </button>
          {savedAt && <span className="hint">Saved</span>}
          <button type="button" className="button-primary" onClick={openChat}>
            <Play size={14} /> Run
          </button>
        </div>
      </div>

      {error && <p className="error">{error}</p>}

      <div className="agent-workspace">
        {paletteOpen && (
          <aside className="agent-palette">
            <div className="agent-palette-label">Nodes</div>
            {PALETTE.map(({ type, label, icon: Icon }) => (
              <div
                key={type}
                className="palette-item"
                draggable
                onDragStart={(e) => onDragStart(e, type)}
                onClick={() => addNode(type)}
              >
                <Icon size={15} />
                {label}
              </div>
            ))}
            <p className="hint">Drag onto the canvas, or click to add.</p>
          </aside>
        )}

        <div className="agent-canvas-area" ref={canvasRef} onDragOver={onDragOver} onDrop={onDrop}>
          {loaded && (
            <ReactFlow
              nodes={nodes}
              edges={edges}
              nodeTypes={nodeTypes}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onConnect={onConnect}
              onNodeClick={(_, node) => setSelectedNodeId(node.id)}
              onPaneClick={() => setSelectedNodeId(null)}
              fitView
            >
              <Background />
              <Controls />
            </ReactFlow>
          )}
        </div>

        {inspectorOpen && (
          <aside className="agent-inspector">
            <div className="agent-settings">
              <div className="agent-palette-label">Agent settings</div>
              <label>
                Environment
                <select value={environment} onChange={(e) => setEnvironment(e.target.value)}>
                  <option value="">None</option>
                  {environments.map((env) => (
                    <option key={env.name} value={env.name}>
                      {env.name}
                    </option>
                  ))}
                </select>
              </label>
              {environment && (
                <p className="hint">
                  Tools available: {environments.find((env) => env.name === environment)?.tools.join(', ') || 'none'}
                </p>
              )}
            </div>
            {!selectedNode && <p className="hint">Select a node on the canvas to configure it.</p>}
            {selectedNode && (
              <>
                <h3>{selectedNode.type} node</h3>
                {selectedNode.type === 'prompt' && (
                  <div className="stacked-form">
                    <label>
                      Model
                      <select
                        value={selectedNode.data.model ?? ''}
                        onChange={(e) => updateSelectedNodeData({ model: e.target.value })}
                      >
                        <option value="">Select a model</option>
                        {models.map((m) => (
                          <option key={m.name} value={m.name}>
                            {m.name}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label>
                      System prompt
                      <textarea
                        rows={4}
                        value={selectedNode.data.systemPrompt ?? ''}
                        onChange={(e) => updateSelectedNodeData({ systemPrompt: e.target.value })}
                      />
                    </label>
                  </div>
                )}
                {selectedNode.type === 'decision' && (
                  <div className="stacked-form">
                    <label>
                      Keyword (routes to "yes" if the previous output contains it)
                      <input
                        type="text"
                        value={selectedNode.data.keyword ?? ''}
                        onChange={(e) => updateSelectedNodeData({ keyword: e.target.value })}
                      />
                    </label>
                  </div>
                )}
                {selectedNode.type === 'tool' && (
                  <div className="stacked-form">
                    <label>
                      Command
                      <textarea
                        rows={3}
                        value={selectedNode.data.command ?? ''}
                        onChange={(e) => updateSelectedNodeData({ command: e.target.value })}
                        placeholder="e.g. cat {{input}}"
                      />
                    </label>
                    <p className="hint">
                      Runs inside the agent's Environment. Use <code>{'{{input}}'}</code> to insert the previous
                      node's output.
                    </p>
                  </div>
                )}
                {(selectedNode.type === 'input' || selectedNode.type === 'output') && (
                  <div className="stacked-form">
                    <label>
                      Label
                      <input
                        type="text"
                        value={selectedNode.data.label ?? ''}
                        onChange={(e) => updateSelectedNodeData({ label: e.target.value })}
                      />
                    </label>
                  </div>
                )}
                {selectedNode.type !== 'input' && (
                  <button type="button" className="danger-button" onClick={deleteSelectedNode}>
                    Delete node
                  </button>
                )}
              </>
            )}
          </aside>
        )}
      </div>

      {chatOpen && (
        <Modal title={`Chat with ${name}`} onClose={closeChat}>
          {!runId && <p className="hint">Starting a run…</p>}
          {runId && (
            <>
              <div className="chat-log">
                {messages.length === 0 && <p className="hint">Say hello to try your agent.</p>}
                {messages.map((m, i) => (
                  <div key={i} className={`chat-message chat-message-${m.role}`}>
                    <span className="chat-role">{m.role}</span>
                    <span>{m.content}</span>
                  </div>
                ))}
                <div ref={messagesEndRef} />
              </div>
              <form className="inline-form" onSubmit={handleSendMessage}>
                <input
                  type="text"
                  placeholder="Type a message…"
                  value={chatInput}
                  onChange={(e) => setChatInput(e.target.value)}
                  disabled={sending}
                  autoFocus
                />
                <button type="submit" disabled={sending || !chatInput.trim()}>
                  {sending ? 'Sending…' : 'Send'}
                </button>
              </form>

              {steps.length > 0 && (
                <>
                  <h3>Live steps</h3>
                  <ul className="event-log">
                    {steps.map((s, i) => (
                      <li key={i}>
                        <span className="event-type">{s.nodeType}</span>
                        <span className="event-data">{s.output}</span>
                      </li>
                    ))}
                  </ul>
                </>
              )}
            </>
          )}
        </Modal>
      )}
    </div>
  )
}

export default AgentEditor
