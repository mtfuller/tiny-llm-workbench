import {
  addEdge,
  Background,
  Controls,
  MiniMap,
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
import {
  BookOpen,
  GitBranch,
  LogIn,
  MessageSquare,
  MousePointerClick,
  PanelLeftClose,
  PanelLeftOpen,
  PanelRightClose,
  PanelRightOpen,
  Play,
  Settings,
  SquareArrowOutUpRight,
  Terminal,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState, type ComponentType, type DragEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  getAgent,
  listEnvironments,
  listKnowledgeBases,
  listModels,
  listTools,
  saveAgent,
  sendAgentMessage,
  startAgentRun,
  stopAgentRun,
  type AgentNodeData,
  type AgentStepEvent,
  type ChatMessage,
  type Environment,
  type KnowledgeBase,
  type Model,
  type NodeType,
  type Tool,
} from '../api'
import { nodeTypes } from '../agentNodes'
import { useEventStream } from '../eventStream'
import Modal from '../Modal'
import { suggestedModels } from '../suggestedModels'
import { insertAtCursor, upstreamVariableOptions, VariableMenuButton } from '../TemplateField'

type FlowNode = Node<AgentNodeData>

// Mirrors the .flow-node-* border colors in index.css so the minimap (and
// the inspector's node-type header) reads as the same graph, just recolored
// consistently everywhere it appears.
const NODE_COLORS: Record<string, string> = {
  prompt: '#2f6fd6',
  tool: '#2f8f6d',
  knowledge: '#7c5cbf',
  output: '#d0447a',
}

function minimapNodeColor(node: FlowNode): string {
  const root = getComputedStyle(document.documentElement)
  switch (node.type) {
    case 'decision':
      return root.getPropertyValue('--warn').trim() || '#b8792f'
    case 'input':
      return root.getPropertyValue('--accent').trim() || '#c1633f'
    default:
      return NODE_COLORS[node.type ?? ''] ?? (root.getPropertyValue('--accent').trim() || '#c1633f')
  }
}

interface NodeMeta {
  label: string
  icon: ComponentType<{ size?: number }>
}

const NODE_META: Record<NodeType, NodeMeta> = {
  input: { label: 'Input', icon: LogIn },
  prompt: { label: 'Prompt', icon: MessageSquare },
  decision: { label: 'Decision', icon: GitBranch },
  tool: { label: 'Tool', icon: Terminal },
  knowledge: { label: 'Knowledge', icon: BookOpen },
  output: { label: 'Output', icon: SquareArrowOutUpRight },
}

const PALETTE: NodeType[] = ['prompt', 'decision', 'tool', 'knowledge', 'output']

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
  const [toolCatalog, setToolCatalog] = useState<Tool[]>([])
  const [knowledgeBases, setKnowledgeBases] = useState<KnowledgeBase[]>([])
  const [environment, setEnvironment] = useState('')
  const [description, setDescription] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [paletteOpen, setPaletteOpen] = useState(true)
  const [inspectorOpen, setInspectorOpen] = useState(true)
  const [settingsOpen, setSettingsOpen] = useState(false)

  const [chatOpen, setChatOpen] = useState(false)
  const [runId, setRunId] = useState<string | null>(null)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [chatInput, setChatInput] = useState('')
  const [sending, setSending] = useState(false)
  const [steps, setSteps] = useState<AgentStepEvent[]>([])
  const messagesEndRef = useRef<HTMLDivElement>(null)

  // Refs for the templatable fields the "insert variable" picker writes
  // into at the current cursor position. toolArgRefs is keyed by parameter
  // name since a tool node's fields are rendered dynamically.
  const systemPromptRef = useRef<HTMLTextAreaElement>(null)
  const promptTemplateRef = useRef<HTMLTextAreaElement>(null)
  const matchTemplateRef = useRef<HTMLTextAreaElement>(null)
  const knowledgeQueryRef = useRef<HTMLTextAreaElement>(null)
  const toolArgRefs = useRef<Map<string, HTMLInputElement>>(new Map())

  useEffect(() => {
    getAgent(name)
      .then((agent) => {
        setNodes(agent.graph.nodes as FlowNode[])
        setEdges(agent.graph.edges as Edge[])
        setEnvironment(agent.environment ?? '')
        setDescription(agent.description ?? '')
        setLoaded(true)
      })
      .catch((err: Error) => setError(err.message))
    listModels()
      .then(setModels)
      .catch(() => setModels([]))
    listEnvironments()
      .then(setEnvironments)
      .catch(() => setEnvironments([]))
    listTools()
      .then(setToolCatalog)
      .catch(() => setToolCatalog([]))
    listKnowledgeBases()
      .then(setKnowledgeBases)
      .catch(() => setKnowledgeBases([]))
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
      const countOfType = nodes.filter((n) => n.type === type).length + 1
      const data: AgentNodeData = {
        name: `${NODE_META[type].label} ${countOfType}`,
        ...(type === 'prompt' ? { model: models[0]?.name } : {}),
      }
      setNodes((nds) => [
        ...nds,
        { id, type, position: position ?? { x: 120 + nds.length * 40, y: 120 + (nds.length % 4) * 90 }, data },
      ])
    },
    [models, nodes, setNodes],
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

  const modelOptions = useMemo(() => {
    const trained = models.map((m) => m.name)
    return Array.from(new Set([...trained, ...suggestedModels]))
  }, [models])

  const boundEnvironment = useMemo(() => environments.find((e) => e.name === environment), [environments, environment])
  // The environment only stores tool NAMES (a live reference into the
  // global catalog) — resolve them here the same way EnvironmentDetail's
  // Playground does, dropping any name that's since been deleted from the
  // catalog rather than erroring.
  const availableTools = useMemo(
    () => (boundEnvironment?.tools ?? []).map((n) => toolCatalog.find((t) => t.name === n)).filter((t): t is Tool => t !== undefined),
    [boundEnvironment, toolCatalog],
  )

  const selectedTool: Tool | undefined = useMemo(
    () => availableTools.find((t) => t.name === selectedNode?.data.toolName),
    [availableTools, selectedNode],
  )

  // Every named node reachable upstream of the currently selected node —
  // what the "insert variable" picker offers for its templatable fields.
  const upstreamOptions = useMemo(
    () => (selectedNode ? upstreamVariableOptions(nodes, edges, selectedNode.id) : []),
    [nodes, edges, selectedNode],
  )

  const handleToolChange = (toolName: string) => {
    updateSelectedNodeData({ toolName, toolArgs: {} })
  }

  const handleToolArgChange = (paramName: string, value: string) => {
    updateSelectedNodeData({ toolArgs: { ...(selectedNode?.data.toolArgs ?? {}), [paramName]: value } })
  }

  const handleSave = async () => {
    const names = nodes.map((n) => n.data.name?.trim()).filter((n): n is string => !!n)
    const duplicate = names.find((n, i) => names.indexOf(n) !== i)
    if (duplicate) {
      setError(`Node names must be unique — "${duplicate}" is used more than once.`)
      return
    }

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
        description || undefined,
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
            {PALETTE.map((type) => {
              const { label, icon: Icon } = NODE_META[type]
              return (
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
              )
            })}
            <p className="hint">Drag onto the canvas, or click to add.</p>

            <div className="agent-palette-footer">
              <button type="button" className="palette-settings-button" onClick={() => setSettingsOpen(true)}>
                <Settings size={15} /> Agent settings
              </button>
            </div>
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
              <Background gap={18} />
              <Controls />
              <MiniMap
                pannable
                zoomable
                nodeColor={minimapNodeColor}
                maskColor="var(--minimap-mask)"
              />
            </ReactFlow>
          )}
        </div>

        {inspectorOpen && (
          <aside className="agent-inspector">
            {!selectedNode && (
              <div className="inspector-empty">
                <MousePointerClick size={28} />
                <p>Select a node on the canvas to configure it.</p>
              </div>
            )}
            {selectedNode && (
              <>
                <div className={`inspector-node-header inspector-node-header-${selectedNode.type}`}>
                  {(() => {
                    const Icon = NODE_META[selectedNode.type as NodeType]?.icon ?? MessageSquare
                    return <Icon size={16} />
                  })()}
                  <span>{NODE_META[selectedNode.type as NodeType]?.label ?? selectedNode.type} node</span>
                </div>

                <div className="inspector-body">
                  <label>
                    Name
                    <input
                      type="text"
                      placeholder={NODE_META[selectedNode.type as NodeType]?.label}
                      value={selectedNode.data.name ?? ''}
                      onChange={(e) => updateSelectedNodeData({ name: e.target.value })}
                    />
                    <span className="field-hint">
                      Reference this node's output elsewhere as {'{{'}
                      {selectedNode.data.name || '…'}
                      {'}}'}.
                    </span>
                  </label>

                  {selectedNode.type === 'prompt' && (
                    <>
                      <label>
                        Model
                        <input
                          type="text"
                          list="agent-model-options"
                          placeholder="mlx-community/Qwen2.5-0.5B-Instruct-4bit"
                          value={selectedNode.data.model ?? ''}
                          onChange={(e) => updateSelectedNodeData({ model: e.target.value })}
                        />
                        <datalist id="agent-model-options">
                          {modelOptions.map((m) => (
                            <option key={m} value={m} />
                          ))}
                        </datalist>
                      </label>

                      <label>
                        System prompt
                        <div className="template-field-row">
                          <textarea
                            ref={systemPromptRef}
                            rows={5}
                            placeholder="You are a helpful assistant…"
                            value={selectedNode.data.systemPrompt ?? ''}
                            onChange={(e) => updateSelectedNodeData({ systemPrompt: e.target.value })}
                          />
                          <VariableMenuButton
                            options={upstreamOptions}
                            onInsert={(snippet) =>
                              insertAtCursor(systemPromptRef.current, selectedNode.data.systemPrompt ?? '', snippet, (next) =>
                                updateSelectedNodeData({ systemPrompt: next }),
                              )
                            }
                          />
                        </div>
                      </label>

                      <label>
                        Prompt template
                        <div className="template-field-row">
                          <textarea
                            ref={promptTemplateRef}
                            rows={4}
                            placeholder="e.g. please solve user problem: {{Input}}"
                            value={selectedNode.data.promptTemplate ?? ''}
                            onChange={(e) => updateSelectedNodeData({ promptTemplate: e.target.value })}
                          />
                          <VariableMenuButton
                            options={upstreamOptions}
                            onInsert={(snippet) =>
                              insertAtCursor(promptTemplateRef.current, selectedNode.data.promptTemplate ?? '', snippet, (next) =>
                                updateSelectedNodeData({ promptTemplate: next }),
                              )
                            }
                          />
                        </div>
                        <span className="field-hint">
                          Leave blank to pass the previous node's output through unchanged as the user turn.
                        </span>
                      </label>

                      <label>
                        Output schema (optional)
                        <textarea
                          className="assertion-json-schema"
                          rows={4}
                          placeholder='{"type": "object", "required": ["city"]}'
                          value={selectedNode.data.outputSchema ?? ''}
                          onChange={(e) => updateSelectedNodeData({ outputSchema: e.target.value })}
                        />
                        <span className="field-hint">
                          If set, the reply must validate against this JSON Schema — the turn fails otherwise.
                          Downstream nodes can then reference {'{{'}
                          {selectedNode.data.name || 'ThisNode'}.property{'}}'}.
                        </span>
                      </label>
                    </>
                  )}

                  {selectedNode.type === 'decision' && (
                    <>
                      <label>
                        Keyword
                        <input
                          type="text"
                          placeholder="e.g. weather"
                          value={selectedNode.data.keyword ?? ''}
                          onChange={(e) => updateSelectedNodeData({ keyword: e.target.value })}
                        />
                        <span className="field-hint">Routes to "yes" if the match text below contains this.</span>
                      </label>

                      <label>
                        Match against (optional)
                        <div className="template-field-row">
                          <textarea
                            ref={matchTemplateRef}
                            rows={3}
                            placeholder="Leave blank to check the previous node's raw output"
                            value={selectedNode.data.matchTemplate ?? ''}
                            onChange={(e) => updateSelectedNodeData({ matchTemplate: e.target.value })}
                          />
                          <VariableMenuButton
                            options={upstreamOptions}
                            onInsert={(snippet) =>
                              insertAtCursor(matchTemplateRef.current, selectedNode.data.matchTemplate ?? '', snippet, (next) =>
                                updateSelectedNodeData({ matchTemplate: next }),
                              )
                            }
                          />
                        </div>
                        <span className="field-hint">
                          Reference a specific node's output or JSON property instead — e.g. {'{{Analyzer.sentiment}}'}.
                        </span>
                      </label>
                    </>
                  )}

                  {selectedNode.type === 'tool' && (
                    <>
                      {!environment && (
                        <p className="hint">
                          This agent has no Environment configured —{' '}
                          <button type="button" className="link-button" onClick={() => setSettingsOpen(true)}>
                            set one in Agent settings
                          </button>{' '}
                          to give it tools to run.
                        </p>
                      )}

                      {environment && (
                        <label>
                          Tool
                          <select value={selectedNode.data.toolName ?? ''} onChange={(e) => handleToolChange(e.target.value)}>
                            <option value="">Select a tool…</option>
                            {availableTools.map((t) => (
                              <option key={t.name} value={t.name}>
                                {t.name}
                              </option>
                            ))}
                          </select>
                        </label>
                      )}

                      {environment && selectedNode.data.toolName && !selectedTool && (
                        <p className="error">
                          Tool "{selectedNode.data.toolName}" isn't on the "{environment}" environment anymore — pick another.
                        </p>
                      )}

                      {selectedTool && (
                        <div className="tool-arg-list">
                          {selectedTool.description && <p className="hint">{selectedTool.description}</p>}
                          <div className="inspector-section-label">Parameters</div>
                          {selectedTool.parameters.length === 0 && <p className="hint">This tool takes no parameters.</p>}
                          {selectedTool.parameters.map((p) => (
                            <div className="tool-arg-row" key={p.name}>
                              <div className="tool-arg-row-header">
                                <span>
                                  {p.name}
                                  {p.required ? ' *' : ''}
                                </span>
                              </div>
                              {p.description && <span className="field-hint">{p.description}</span>}
                              {p.type === 'boolean' ? (
                                <select
                                  value={selectedNode.data.toolArgs?.[p.name] ?? ''}
                                  onChange={(e) => handleToolArgChange(p.name, e.target.value)}
                                >
                                  <option value="">—</option>
                                  <option value="true">true</option>
                                  <option value="false">false</option>
                                </select>
                              ) : (
                                <div className="template-field-row">
                                  <input
                                    type="text"
                                    ref={(el) => {
                                      if (el) toolArgRefs.current.set(p.name, el)
                                      else toolArgRefs.current.delete(p.name)
                                    }}
                                    placeholder={p.type === 'number' ? `${p.name} (number or {{...}})` : p.name}
                                    value={selectedNode.data.toolArgs?.[p.name] ?? ''}
                                    onChange={(e) => handleToolArgChange(p.name, e.target.value)}
                                  />
                                  <VariableMenuButton
                                    options={upstreamOptions}
                                    onInsert={(snippet) =>
                                      insertAtCursor(
                                        toolArgRefs.current.get(p.name) ?? null,
                                        selectedNode.data.toolArgs?.[p.name] ?? '',
                                        snippet,
                                        (next) => handleToolArgChange(p.name, next),
                                      )
                                    }
                                  />
                                </div>
                              )}
                            </div>
                          ))}
                        </div>
                      )}
                    </>
                  )}

                  {selectedNode.type === 'knowledge' && (
                    <>
                      <label>
                        Knowledge base
                        <select
                          value={selectedNode.data.knowledgeBaseName ?? ''}
                          onChange={(e) => updateSelectedNodeData({ knowledgeBaseName: e.target.value })}
                        >
                          <option value="">Select a knowledge base…</option>
                          {knowledgeBases.map((kb) => (
                            <option key={kb.name} value={kb.name}>
                              {kb.name}
                            </option>
                          ))}
                        </select>
                        {knowledgeBases.length === 0 && (
                          <span className="field-hint">
                            No knowledge bases yet — create one on the <Link to="/knowledge">Knowledge</Link> page.
                          </span>
                        )}
                      </label>

                      <label>
                        Query (optional)
                        <div className="template-field-row">
                          <textarea
                            ref={knowledgeQueryRef}
                            rows={3}
                            placeholder="Leave blank to query with the previous node's raw output"
                            value={selectedNode.data.knowledgeQuery ?? ''}
                            onChange={(e) => updateSelectedNodeData({ knowledgeQuery: e.target.value })}
                          />
                          <VariableMenuButton
                            options={upstreamOptions}
                            onInsert={(snippet) =>
                              insertAtCursor(knowledgeQueryRef.current, selectedNode.data.knowledgeQuery ?? '', snippet, (next) =>
                                updateSelectedNodeData({ knowledgeQuery: next }),
                              )
                            }
                          />
                        </div>
                        <span className="field-hint">
                          Matched records are joined and passed downstream as this node's output — a deterministic
                          keyword match against title and content, not embeddings.
                        </span>
                      </label>
                    </>
                  )}
                </div>

                {selectedNode.type !== 'input' && (
                  <div className="inspector-footer">
                    <button type="button" className="danger-button" onClick={deleteSelectedNode}>
                      <Trash2 size={14} /> Delete node
                    </button>
                  </div>
                )}
              </>
            )}
          </aside>
        )}
      </div>

      {settingsOpen && (
        <AgentSettingsModal
          environment={environment}
          environmentOptions={environments}
          description={description}
          onChangeEnvironment={setEnvironment}
          onChangeDescription={setDescription}
          onClose={() => setSettingsOpen(false)}
        />
      )}

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

interface AgentSettingsModalProps {
  environment: string
  environmentOptions: Environment[]
  description: string
  onChangeEnvironment: (value: string) => void
  onChangeDescription: (value: string) => void
  onClose: () => void
}

function AgentSettingsModal({
  environment,
  environmentOptions,
  description,
  onChangeEnvironment,
  onChangeDescription,
  onClose,
}: AgentSettingsModalProps) {
  const selected = environmentOptions.find((e) => e.name === environment)

  return (
    <Modal title="Agent settings" onClose={onClose}>
      <div className="stacked-form">
        <label>
          Environment
          <select value={environment} onChange={(e) => onChangeEnvironment(e.target.value)}>
            <option value="">None</option>
            {environmentOptions.map((env) => (
              <option key={env.name} value={env.name}>
                {env.name}
              </option>
            ))}
          </select>
        </label>
        {selected && (
          <p className="hint">Tools available: {selected.tools.length > 0 ? selected.tools.join(', ') : 'none'}</p>
        )}

        <label>
          Description
          <textarea
            rows={3}
            placeholder="What does this agent do?"
            value={description}
            onChange={(e) => onChangeDescription(e.target.value)}
          />
        </label>

        <div className="row-actions confirm-actions">
          <button type="button" onClick={onClose}>
            Done
          </button>
        </div>
      </div>
    </Modal>
  )
}

export default AgentEditor
