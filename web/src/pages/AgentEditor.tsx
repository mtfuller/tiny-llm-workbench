import {
  addEdge,
  Background,
  Controls,
  MarkerType,
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
  AlertCircle,
  AlertTriangle,
  BookOpen,
  Bot,
  Bug,
  ChevronDown,
  ChevronRight,
  Database,
  IterationCw,
  Loader2,
  LogIn,
  Megaphone,
  MessageSquare,
  MousePointerClick,
  PanelLeftClose,
  PanelLeftOpen,
  PanelRightClose,
  PanelRightOpen,
  Play,
  Plus,
  Repeat,
  RotateCcw,
  Route,
  Settings,
  SkipForward,
  Split,
  Square,
  Terminal,
  Trash2,
  X,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState, type ComponentType, type DragEvent, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  getAgent,
  listKnowledgeBases,
  listModels,
  listTools,
  listWorkspaces,
  retryAgentDebugRun,
  saveAgent,
  sendAgentMessage,
  sendAgentDebugMessage,
  startAgentDebugRun,
  startAgentRun,
  stepAgentDebugRun,
  stopAgentDebugRun,
  stopAgentRun,
  type AgentGraph,
  type AgentMessageEvent,
  type AgentNodeData,
  type AgentStepEvent,
  type ChatMessage,
  type DebugState,
  type KnowledgeBase,
  type Model,
  type NodeType,
  type Tool,
  type Workspace,
} from '../api'
import { nodeTypes } from '../agentNodes'
import { validateGraph } from '../agentValidation'
import { useEventStream } from '../eventStream'
import Expandable from '../Expandable'
import IconButton from '../IconButton'
import LineNumberedTextarea from '../LineNumberedTextarea'
import MultiPickList from '../MultiPickList'
import SchemaBuilder from '../SchemaBuilder'
import Modal from '../Modal'
import ModelCombobox from '../ModelCombobox'
import { insertAtCursor, upstreamVariableOptions, VariableMenuButton } from '../TemplateField'
import { useResizableSidebar } from '../useResizableSidebar'

type FlowNode = Node<AgentNodeData>

// ChatEntry is a chat-modal message: a normal user/assistant turn, or an
// assistant message streamed from a "say" node (kind "progress" = dimmed,
// collapsible narration; "final" = the definitive reply).
type ChatEntry = ChatMessage & { kind?: 'progress' | 'final' }

// Mirrors the .flow-node-* border colors in index.css so the minimap (and
// the inspector's node-type header) reads as the same graph, just recolored
// consistently everywhere it appears.
const NODE_COLORS: Record<string, string> = {
  prompt: '#2f6fd6',
  tool: '#2f8f6d',
  knowledge: '#7c5cbf',
  switch: '#4b3fae',
  loop_start: '#2f8f8f',
  loop_end: '#2f8f8f',
  state: '#5f6b7a',
  say: '#c25b7c',
  agent: '#b5468a',
}

function minimapNodeColor(node: FlowNode): string {
  const root = getComputedStyle(document.documentElement)
  switch (node.type) {
    case 'condition':
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
  condition: { label: 'Condition', icon: Split },
  switch: { label: 'Switch', icon: Route },
  loop_start: { label: 'Loop start', icon: Repeat },
  loop_end: { label: 'Loop end', icon: IterationCw },
  state: { label: 'State', icon: Database },
  say: { label: 'Say', icon: Megaphone },
  tool: { label: 'Tool', icon: Terminal },
  agent: { label: 'Agent', icon: Bot },
  knowledge: { label: 'Knowledge', icon: BookOpen },
}

const PALETTE: NodeType[] = ['prompt', 'condition', 'switch', 'loop_start', 'loop_end', 'state', 'say', 'tool', 'agent', 'knowledge']

// Arrowheads on every edge so flow direction reads at a glance.
const ARROW_EDGE = { markerEnd: { type: MarkerType.ArrowClosed } } as const

let nodeCounter = 0
function newNodeId(type: string): string {
  nodeCounter += 1
  return `${type}-${Date.now()}-${nodeCounter}`
}

// elapsedLabel renders "M:SS" for the time since startedAt. tick is an unused
// arg that just forces a re-render each second while a debug step runs.
function elapsedLabel(startedAt: number, _tick: number): string {
  const s = Math.max(0, Math.floor((Date.now() - startedAt) / 1000))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
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
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [toolCatalog, setToolCatalog] = useState<Tool[]>([])
  const [knowledgeBases, setKnowledgeBases] = useState<KnowledgeBase[]>([])
  // The agent's access: a bound TEST workspace + the tool/KB pools nodes pick from.
  const [workspace, setWorkspace] = useState('')
  const [agentTools, setAgentTools] = useState<string[]>([])
  const [agentKnowledgeBases, setAgentKnowledgeBases] = useState<string[]>([])
  const [description, setDescription] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [paletteOpen, setPaletteOpen] = useState(true)
  const [leftTab, setLeftTab] = useState<'nodes' | 'debug'>('nodes')
  const [inspectorOpen, setInspectorOpen] = useState(true)
  // The right sidebar shows the node inspector, or — during a debug session —
  // the live activity feed. A tab bar switches between them.
  const [inspectorTab, setInspectorTab] = useState<'inspector' | 'activity'>('inspector')
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [issuesOpen, setIssuesOpen] = useState(true)

  const [chatOpen, setChatOpen] = useState(false)
  const [runId, setRunId] = useState<string | null>(null)
  // Optional per-run TEST workspace override for chat/debug — '' means "use
  // the agent's bound workspace".
  const [runWorkspace, setRunWorkspace] = useState('')
  const [messages, setMessages] = useState<ChatEntry[]>([])
  const [chatInput, setChatInput] = useState('')
  const [sending, setSending] = useState(false)
  const [steps, setSteps] = useState<AgentStepEvent[]>([])
  const messagesEndRef = useRef<HTMLDivElement>(null)
  // True once a "say" node marked final streamed this turn — so the HTTP
  // reply (which carries the same text) isn't appended a second time.
  const streamedFinalRef = useRef(false)

  const [debugState, setDebugState] = useState<DebugState | null>(null)
  const [debugInput, setDebugInput] = useState('')
  const [debugBusy, setDebugBusy] = useState(false)
  const [debugStarting, setDebugStarting] = useState(false)
  const [debugError, setDebugError] = useState<string | null>(null)
  // Live feed for the debug panel: agent.step / agent.message events for the
  // active debug run, so a step that's waiting on the model isn't a silent
  // freeze. Reset at the start of each turn.
  const [debugFeed, setDebugFeed] = useState<AgentStepEvent[]>([])
  // When the in-flight step started, for the ticking "running… 0:42" label.
  const [stepStartedAt, setStepStartedAt] = useState<number | null>(null)
  const [nowTick, setNowTick] = useState(0)
  // Tracks the latest debugState for the unmount-cleanup effect below,
  // which otherwise would only ever see the (always null) value captured
  // when the effect itself was first set up.
  const debugStateRef = useRef<DebugState | null>(null)
  useEffect(() => {
    debugStateRef.current = debugState
  }, [debugState])
  useEffect(() => {
    return () => {
      if (debugStateRef.current) void stopAgentDebugRun(debugStateRef.current.id).catch(() => {})
    }
  }, [])

  // Refs for the templatable fields the "insert variable" picker writes
  // into at the current cursor position. toolArgRefs is keyed by parameter
  // name since a tool node's fields are rendered dynamically.
  const systemPromptRef = useRef<HTMLTextAreaElement>(null)
  const promptTemplateRef = useRef<HTMLTextAreaElement>(null)
  const matchTemplateRef = useRef<HTMLTextAreaElement>(null)
  const knowledgeQueryRef = useRef<HTMLTextAreaElement>(null)
  const stateValueRef = useRef<HTMLTextAreaElement>(null)
  const sayTemplateRef = useRef<HTMLTextAreaElement>(null)
  const agentInstructionsRef = useRef<HTMLTextAreaElement>(null)
  const toolArgRefs = useRef<Map<string, HTMLInputElement>>(new Map())

  useEffect(() => {
    getAgent(name)
      .then((agent) => {
        setNodes(agent.graph.nodes as FlowNode[])
        // Nodes that render an out/fail handle pair (a JSON schema is set) need
        // their default-branch edges anchored to the "out" handle, since the
        // saved payload canonicalises that branch to the unnamed handle.
        const schemaNodeIds = new Set(
          agent.graph.nodes
            .filter((n) => (n.type === 'prompt' && n.data.outputSchema?.trim()) || (n.type === 'agent' && n.data.agentOutputSchema?.trim()))
            .map((n) => n.id),
        )
        setEdges(
          (agent.graph.edges as Edge[]).map((e) => ({
            ...e,
            sourceHandle: e.sourceHandle || (schemaNodeIds.has(e.source) ? 'out' : e.sourceHandle),
            ...ARROW_EDGE,
          })),
        )
        setWorkspace(agent.workspace ?? '')
        setAgentTools(agent.tools ?? [])
        setAgentKnowledgeBases(agent.knowledgeBases ?? [])
        setDescription(agent.description ?? '')
        setLoaded(true)
      })
      .catch((err: Error) => setError(err.message))
    listModels()
      .then(setModels)
      .catch(() => setModels([]))
    listWorkspaces()
      .then(setWorkspaces)
      .catch(() => setWorkspaces([]))
    listTools()
      .then(setToolCatalog)
      .catch(() => setToolCatalog([]))
    listKnowledgeBases()
      .then(setKnowledgeBases)
      .catch(() => setKnowledgeBases([]))
  }, [name, setNodes, setEdges])

  const debugRunId = debugState?.id ?? null

  useEffect(() => {
    return subscribe('agent.step', (event) => {
      const step = JSON.parse(event.data) as AgentStepEvent
      if (step.runId === runId) setSteps((prev) => [...prev, step])
      if (step.runId === debugRunId) setDebugFeed((prev) => [...prev, step])
    })
  }, [subscribe, runId, debugRunId])

  // "say" nodes stream user-facing messages here as the turn runs.
  useEffect(() => {
    return subscribe('agent.message', (event) => {
      const msg = JSON.parse(event.data) as AgentMessageEvent
      if (msg.runId === debugRunId) {
        setDebugFeed((prev) => [
          ...prev,
          { runId: msg.runId, nodeId: 'say', nodeType: 'say', output: `${msg.kind}: ${msg.content}` },
        ])
      }
      if (msg.runId !== runId) return
      if (msg.kind === 'final') streamedFinalRef.current = true
      setMessages((prev) => [
        ...prev,
        { role: 'assistant', content: msg.content, timestamp: new Date().toISOString(), kind: msg.kind },
      ])
    })
  }, [subscribe, runId, debugRunId])

  // Tick once a second while a debug step is in flight, so the elapsed-time
  // label updates.
  useEffect(() => {
    if (stepStartedAt === null) return
    const id = setInterval(() => setNowTick((n) => n + 1), 1000)
    return () => clearInterval(id)
  }, [stepStartedAt])

  useEffect(() => {
    if (chatOpen) messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, chatOpen])

  // Keep edge source handles in sync as a node's JSON schema is toggled on/off:
  // a schema'd prompt/agent node renders an out/fail handle pair, so its
  // default-branch edges must anchor to "out"; without a schema they go back to
  // the single unnamed handle.
  useEffect(() => {
    // The out/fail handle pair only exists on prompt/agent nodes; scope the
    // rewrite to those so a switch node's own case handles are never touched.
    const schemaCapable = new Set(nodes.filter((n) => n.type === 'prompt' || n.type === 'agent').map((n) => n.id))
    const schemaIds = new Set(
      nodes
        .filter((n) => (n.type === 'prompt' && n.data.outputSchema?.trim()) || (n.type === 'agent' && n.data.agentOutputSchema?.trim()))
        .map((n) => n.id),
    )
    setEdges((eds) => {
      let changed = false
      const next = eds.map((e) => {
        if (schemaIds.has(e.source) && !e.sourceHandle) {
          changed = true
          return { ...e, sourceHandle: 'out' }
        }
        if (schemaCapable.has(e.source) && !schemaIds.has(e.source) && (e.sourceHandle === 'out' || e.sourceHandle === 'fail')) {
          changed = true
          return { ...e, sourceHandle: undefined }
        }
        return e
      })
      return changed ? next : eds
    })
  }, [nodes, setEdges])

  const onConnect = useCallback(
    (connection: Connection) => setEdges((eds) => addEdge({ ...connection, id: `e-${Date.now()}`, ...ARROW_EDGE }, eds)),
    [setEdges],
  )

  const addNode = useCallback(
    (type: NodeType, position?: { x: number; y: number }) => {
      const id = newNodeId(type)
      const countOfType = nodes.filter((n) => n.type === type).length + 1
      // Click-to-add stacks nodes top-to-bottom (the canvas flows downward);
      // a drop uses the pointer position.
      const at = position ?? { x: 240 + (nodes.length % 3) * 48, y: 80 + nodes.length * 96 }
      const perTypeDefaults: Partial<Record<NodeType, Partial<AgentNodeData>>> = {
        prompt: { model: models[0]?.name },
        condition: { conditionType: 'contains' },
        switch: { switchCases: [{ value: '' }] },
        loop_start: { loopMaxIterations: 5 },
        loop_end: { loopStartName: nodes.find((n) => n.type === 'loop_start')?.data.name ?? '' },
        state: { stateOp: 'append' },
        say: { sayTemplate: '' },
        agent: { agentModel: models[0]?.name, agentMaxIterations: 6, agentTools: [], agentKnowledgeBases: [] },
      }
      const perType: Partial<AgentNodeData> = perTypeDefaults[type] ?? {}
      const data: AgentNodeData = { name: `${NODE_META[type].label} ${countOfType}`, ...perType }
      const newNodes: FlowNode[] = [{ id, type, position: at, data }]

      // Dropping a Loop start gives you the matching Loop end too, pre-paired —
      // the loop reads as a bracket you close, not two nodes to wire by hand.
      if (type === 'loop_start') {
        const endCount = nodes.filter((n) => n.type === 'loop_end').length + 1
        newNodes.push({
          id: newNodeId('loop_end'),
          type: 'loop_end',
          position: { x: at.x, y: at.y + 220 },
          data: { name: `${NODE_META.loop_end.label} ${endCount}`, loopStartName: data.name },
        })
      }
      setNodes((nds) => [...nds, ...newNodes])
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
    const target = nodes.find((n) => n.id === selectedNodeId)
    // Deleting a Loop start also removes the Loop end(s) paired to it — the
    // mirror of the pair being created together.
    const alsoRemove = new Set([selectedNodeId])
    if (target?.type === 'loop_start' && target.data.name) {
      for (const n of nodes) {
        if (n.type === 'loop_end' && n.data.loopStartName === target.data.name) alsoRemove.add(n.id)
      }
    }
    setNodes((nds) => nds.filter((n) => !alsoRemove.has(n.id)))
    setEdges((eds) => eds.filter((e) => !alsoRemove.has(e.source) && !alsoRemove.has(e.target)))
    setSelectedNodeId(null)
  }

  const updateSelectedNodeData = (patch: Partial<AgentNodeData>) => {
    setNodes((nds) => nds.map((n) => (n.id === selectedNodeId ? { ...n, data: { ...n.data, ...patch } } : n)))
  }

  const selectedNode = nodes.find((n) => n.id === selectedNodeId)

  const modelNames = useMemo(() => models.map((m) => m.name), [models])

  // availableTools resolves the agent's Tools set (names) against the global
  // catalog, dropping any name that's since been deleted rather than
  // erroring. This is the pool a tool / agent node picks from.
  const availableTools = useMemo(
    () => agentTools.map((n) => toolCatalog.find((t) => t.name === n)).filter((t): t is Tool => t !== undefined),
    [agentTools, toolCatalog],
  )
  // The agent's knowledge-base set resolved against all bases.
  const availableKnowledgeBases = useMemo(
    () => agentKnowledgeBases.map((n) => knowledgeBases.find((k) => k.name === n)).filter((k): k is KnowledgeBase => k !== undefined),
    [agentKnowledgeBases, knowledgeBases],
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

  // buildGraphPayload maps the canvas's live React Flow state into the
  // plain AgentGraph shape the backend expects — shared by Save and
  // StartDebugRun (the latter deliberately debugs this exact, possibly
  // unsaved, in-progress graph rather than requiring a save first).
  const buildGraphPayload = (): AgentGraph => {
    // Only a genuinely branching node emits a meaningful sourceHandle
    // ("pass"/"fail", "body"/"done", switch cases + "default", or a schema'd
    // prompt/agent's out/fail pair). Any other edge's sourceHandle is noise
    // — canvas leftovers from a removed schema, a mis-drawn connection — and
    // must be stripped, or the engine can't find the linear next edge and
    // silently dead-ends the turn (echoing the node's own input back).
    const branchSources = new Set(
      nodes
        .filter(
          (n) =>
            n.type === 'condition' ||
            n.type === 'switch' ||
            n.type === 'loop_start' ||
            (n.type === 'prompt' && n.data.outputSchema?.trim()) ||
            (n.type === 'agent' && n.data.agentOutputSchema?.trim()),
        )
        .map((n) => n.id),
    )
    return {
      nodes: nodes.map((n) => ({ id: n.id, type: n.type as NodeType, position: n.position, data: n.data })),
      edges: edges.map((e) => ({
        id: e.id,
        source: e.source,
        // The schema "out" handle is just the visual default branch — the
        // engine's pass path is the unnamed handle, so canonicalise it away.
        sourceHandle:
          branchSources.has(e.source) && e.sourceHandle && e.sourceHandle !== 'out' ? e.sourceHandle : undefined,
        target: e.target,
      })),
    }
  }

  const handleSave = async () => {
    // Save is never blocked by validation — you can save work in progress;
    // only Run and Debug are gated (see runBlocked).
    setSaving(true)
    setError(null)
    try {
      await saveAgent(name, buildGraphPayload(), {
        workspace: workspace || undefined,
        tools: agentTools,
        knowledgeBases: agentKnowledgeBases,
        description: description || undefined,
      })
      setSavedAt(Date.now())
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const openChat = () => {
    if (runBlocked) return
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
    if (runBlocked) return
    setError(null)
    try {
      const run = await startAgentRun(name, runWorkspace || undefined)
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
    streamedFinalRef.current = false
    try {
      const reply = await sendAgentMessage(runId, userMessage.content)
      // If a "say" node marked final already streamed the reply in, don't
      // append it again — its content is identical.
      if (!streamedFinalRef.current) setMessages((prev) => [...prev, reply])
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSending(false)
    }
  }

  const openDebug = () => {
    if (runBlocked) return
    setPaletteOpen(true)
    setLeftTab('debug')
    // The activity feed lives in the right sidebar during a debug session.
    setInspectorOpen(true)
    setInspectorTab('activity')
    if (!debugState) void handleStartDebug()
  }

  const stopDebugging = () => {
    if (debugState) void stopAgentDebugRun(debugState.id).catch(() => {})
    setDebugState(null)
    setDebugInput('')
    setDebugError(null)
    setDebugFeed([])
    setStepStartedAt(null)
    setInspectorTab('inspector')
  }

  const handleStartDebug = async () => {
    if (runBlocked) {
      setDebugError('Fix the graph issues listed above the canvas before debugging.')
      return
    }

    setDebugStarting(true)
    setDebugError(null)
    // The activity feed lives in the right sidebar during a debug session.
    setInspectorOpen(true)
    setInspectorTab('activity')
    try {
      const state = await startAgentDebugRun(name, buildGraphPayload(), {
        workspace: runWorkspace || workspace || undefined,
        tools: agentTools,
        knowledgeBases: agentKnowledgeBases,
      })
      setDebugState(state)
    } catch (err) {
      setDebugError((err as Error).message)
    } finally {
      setDebugStarting(false)
    }
  }

  const handleSendDebugMessage = async (e: FormEvent) => {
    e.preventDefault()
    if (!debugState || !debugInput.trim()) return

    setDebugBusy(true)
    setDebugError(null)
    setDebugFeed([]) // a fresh turn
    try {
      const state = await sendAgentDebugMessage(debugState.id, debugInput.trim())
      setDebugState(state)
      setDebugInput('')
    } catch (err) {
      setDebugError((err as Error).message)
    } finally {
      setDebugBusy(false)
    }
  }

  const runDebugAction = async (action: () => Promise<DebugState>) => {
    if (!debugState) return
    setDebugBusy(true)
    setDebugError(null)
    setStepStartedAt(Date.now())
    setNowTick(0)
    try {
      setDebugState(await action())
    } catch (err) {
      setDebugError((err as Error).message)
    } finally {
      setDebugBusy(false)
      setStepStartedAt(null)
    }
  }

  const handleDebugStep = () => runDebugAction(() => stepAgentDebugRun(debugState!.id))
  const handleDebugRetry = () => runDebugAction(() => retryAgentDebugRun(debugState!.id))

  // nodesForCanvas overlays the active debug session's pending/last-executed
  // node onto a copy of the real canvas nodes, purely for rendering — Save
  // and StartDebugRun both read from `nodes` directly, so this highlight
  // flag never reaches the backend.
  const nodesForCanvas = useMemo(() => {
    if (!debugState) return nodes
    return nodes.map((n) => {
      const debugHighlight: 'pending' | 'executed' | undefined =
        debugState.pendingNodeId === n.id ? 'pending' : debugState.lastStep?.nodeId === n.id ? 'executed' : undefined
      return debugHighlight ? { ...n, data: { ...n.data, debugHighlight } } : n
    })
  }, [nodes, debugState])

  // edgesForCanvas adds a dashed "loops back" edge from every loop_end to its
  // paired loop_start. The engine derives this jump from the pairing (it's not
  // a stored edge), so these are display-only and stripped from the payload.
  const edgesForCanvas = useMemo(() => {
    const startIdByName = new Map(nodes.filter((n) => n.type === 'loop_start' && n.data.name).map((n) => [n.data.name, n.id]))
    const backEdges: Edge[] = []
    for (const n of nodes) {
      if (n.type !== 'loop_end') continue
      const targetId =
        startIdByName.get(n.data.loopStartName ?? '') ??
        (startIdByName.size === 1 && !n.data.loopStartName ? [...startIdByName.values()][0] : undefined)
      if (!targetId) continue
      backEdges.push({
        id: `loopback-${n.id}`,
        source: n.id,
        target: targetId,
        animated: true,
        style: { strokeDasharray: '5 5', stroke: 'var(--warn)' },
        markerEnd: { type: MarkerType.ArrowClosed, color: 'var(--warn)' },
      })
    }
    return [...edges, ...backEdges]
  }, [nodes, edges])

  // Live graph validation — recomputed on every edit. Any `error` disables
  // Run and Debug; `warning`s are advisory. See agentValidation.ts.
  const issues = useMemo(
    () => validateGraph({ nodes, edges, workspace, agentTools, agentKnowledgeBases, toolCatalog, knowledgeBases, workspaces }),
    [nodes, edges, workspace, agentTools, agentKnowledgeBases, toolCatalog, knowledgeBases, workspaces],
  )
  const errorCount = issues.filter((i) => i.severity === 'error').length
  const warnCount = issues.length - errorCount
  const runBlocked = errorCount > 0

  const focusIssue = (nodeId?: string) => {
    if (!nodeId) return
    setSelectedNodeId(nodeId)
    setInspectorOpen(true)
  }

  // Drag-resizable workspace sidebars (widths persist to localStorage).
  const palette = useResizableSidebar('tlw.agentPaletteWidth', 190, { min: 150, max: 760, side: 'left' })
  const inspector = useResizableSidebar('tlw.agentInspectorWidth', 300, { min: 220, max: 640, side: 'right' })

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
          <button
            type="button"
            onClick={openDebug}
            disabled={runBlocked}
            title={runBlocked ? `Fix ${errorCount} graph issue${errorCount === 1 ? '' : 's'} before debugging` : undefined}
          >
            <Bug size={14} /> Debug
          </button>
          <button
            type="button"
            className="button-primary"
            onClick={openChat}
            disabled={runBlocked}
            title={runBlocked ? `Fix ${errorCount} graph issue${errorCount === 1 ? '' : 's'} before running` : undefined}
          >
            <Play size={14} /> Run
          </button>
        </div>
      </div>

      {error && <p className="error">{error}</p>}

      {issues.length > 0 && (
        <div className={`agent-issues${runBlocked ? ' agent-issues-blocking' : ''}`}>
          <button
            type="button"
            className="agent-issues-summary"
            onClick={() => setIssuesOpen((o) => !o)}
            aria-expanded={issuesOpen}
          >
            {runBlocked ? <AlertCircle size={14} /> : <AlertTriangle size={14} />}
            <span>
              {runBlocked
                ? `${errorCount} issue${errorCount === 1 ? '' : 's'} to fix — Run and Debug are disabled`
                : `${warnCount} warning${warnCount === 1 ? '' : 's'}`}
              {runBlocked && warnCount > 0 && ` (and ${warnCount} warning${warnCount === 1 ? '' : 's'})`}
            </span>
            {issuesOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>
          {issuesOpen && (
            <ul className="agent-issue-list">
              {issues.map((issue, i) => (
                <li
                  key={i}
                  className={`agent-issue agent-issue-${issue.severity}${issue.nodeId ? ' agent-issue-clickable' : ''}`}
                  onClick={() => focusIssue(issue.nodeId)}
                >
                  {issue.severity === 'error' ? <AlertCircle size={13} /> : <AlertTriangle size={13} />}
                  <span>{issue.message}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      <div className="agent-workspace">
        {paletteOpen && (
          <aside
            className={`agent-palette${leftTab === 'debug' ? ' agent-palette-wide' : ''}`}
            style={{ width: palette.width }}
          >
            <div className="tab-bar">
              <button
                type="button"
                className={`tab-button${leftTab === 'nodes' ? ' tab-button-active' : ''}`}
                onClick={() => setLeftTab('nodes')}
              >
                Nodes
              </button>
              <button type="button" className={`tab-button${leftTab === 'debug' ? ' tab-button-active' : ''}`} onClick={openDebug}>
                Debug
              </button>
            </div>

            {leftTab === 'nodes' && (
              <>
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
              </>
            )}

            {leftTab === 'debug' && (
              <div className="agent-debug-panel">
                {!debugState && debugStarting && <p className="hint">Starting a debug session…</p>}
                {!debugState && !debugStarting && (
                  <button type="button" className="button-primary" onClick={handleStartDebug} disabled={runBlocked}>
                    <Bug size={14} /> Start debugging
                  </button>
                )}
                {!debugState && !debugStarting && runBlocked && (
                  <p className="hint">
                    {errorCount} graph issue{errorCount === 1 ? '' : 's'} must be fixed first — see the panel above the
                    canvas.
                  </p>
                )}
                {debugError && <p className="error">{debugError}</p>}
                {debugState && (
                  <>
                    <div className="chat-log agent-debug-log">
                      {debugState.messages.length === 0 && (
                        <p className="hint">Send a message to start a turn, then Step through each node to watch it play out.</p>
                      )}
                      {debugState.messages.map((m, i) => (
                        <div key={i} className={`chat-message chat-message-${m.role}`}>
                          <span className="chat-role">{m.role}</span>
                          <span>{m.content}</span>
                        </div>
                      ))}
                    </div>

                    <form className="stacked-form" onSubmit={handleSendDebugMessage}>
                      <input
                        type="text"
                        placeholder="Type a message…"
                        value={debugInput}
                        onChange={(e) => setDebugInput(e.target.value)}
                        disabled={debugBusy || !!debugState.pendingNodeId}
                      />
                      <button type="submit" disabled={debugBusy || !debugInput.trim() || !!debugState.pendingNodeId}>
                        {debugBusy ? 'Sending…' : 'Send'}
                      </button>
                    </form>

                    {debugState.pendingNodeId && (
                      <p className="field-hint">
                        Next: <code>{debugState.pendingNodeType}</code> — highlighted with a dashed outline on the canvas.
                      </p>
                    )}
                    {!debugState.pendingNodeId && debugState.lastStep && debugState.messages.length > 0 && (
                      <p className="field-hint">Turn finished — send another message, or retry the last node for a different reply.</p>
                    )}

                    <div className="row-actions">
                      {debugState.lastStep && (
                        <button
                          type="button"
                          className="icon-button"
                          title={`Retry ${debugState.lastStep.nodeType}`}
                          aria-label="Retry last node"
                          onClick={handleDebugRetry}
                          disabled={debugBusy}
                        >
                          <RotateCcw size={15} />
                        </button>
                      )}
                      {debugState.pendingNodeId && (
                        <button type="button" className="button-primary" onClick={handleDebugStep} disabled={debugBusy}>
                          <SkipForward size={14} /> Step
                        </button>
                      )}
                    </div>

                    {debugBusy && stepStartedAt !== null && (
                      <div className="agent-debug-running">
                        <Loader2 size={14} className="chat-modal-spinner" />
                        <span>
                          Running <code>{debugState.pendingNodeType || debugState.lastStep?.nodeType || 'node'}</code> ·{' '}
                          {elapsedLabel(stepStartedAt, nowTick)}
                        </span>
                      </div>
                    )}
                    {debugBusy && stepStartedAt !== null && Math.floor((Date.now() - stepStartedAt) / 1000) > 8 && (
                      <p className="field-hint">
                        The first model call loads (and if it's not cached, downloads) the model — this can take a
                        while.
                      </p>
                    )}

                    {debugFeed.length > 0 && (
                      <button
                        type="button"
                        className="link-button"
                        onClick={() => {
                          setInspectorOpen(true)
                          setInspectorTab('activity')
                        }}
                      >
                        {debugFeed.length} activity event{debugFeed.length === 1 ? '' : 's'} → Activity panel
                      </button>
                    )}

                    <div className="agent-palette-footer">
                      <button type="button" className="palette-settings-button" onClick={stopDebugging}>
                        <Square size={15} /> Stop debugging
                      </button>
                    </div>
                  </>
                )}
              </div>
            )}
          </aside>
        )}
        {paletteOpen && <div {...palette.resizerProps} title="Drag to resize · double-click to reset" />}

        <div className="agent-canvas-area" ref={canvasRef} onDragOver={onDragOver} onDrop={onDrop}>
          {loaded && (
            <ReactFlow
              nodes={nodesForCanvas}
              edges={edgesForCanvas}
              nodeTypes={nodeTypes}
              defaultEdgeOptions={ARROW_EDGE}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onConnect={onConnect}
              onNodeClick={(_, node) => {
                setSelectedNodeId(node.id)
                setInspectorTab('inspector')
              }}
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

        {inspectorOpen && <div {...inspector.resizerProps} title="Drag to resize · double-click to reset" />}
        {inspectorOpen && (
          <aside className="agent-inspector" style={{ width: inspector.width }}>
            {debugState && (
              <div className="tab-bar">
                <button
                  type="button"
                  className={`tab-button${inspectorTab === 'inspector' ? ' tab-button-active' : ''}`}
                  onClick={() => setInspectorTab('inspector')}
                >
                  Inspector
                </button>
                <button
                  type="button"
                  className={`tab-button${inspectorTab === 'activity' ? ' tab-button-active' : ''}`}
                  onClick={() => setInspectorTab('activity')}
                >
                  Activity{debugFeed.length > 0 ? ` (${debugFeed.length})` : ''}
                </button>
              </div>
            )}

            {debugState && inspectorTab === 'activity' && (
              <DebugActivity feed={debugFeed} lastStep={debugState.lastStep} pending={!!debugState.pendingNodeId} />
            )}

            {(!debugState || inspectorTab === 'inspector') && !selectedNode && (
              <div className="inspector-empty">
                <MousePointerClick size={28} />
                <p>Select a node on the canvas to configure it.</p>
              </div>
            )}
            {(!debugState || inspectorTab === 'inspector') && selectedNode && (
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
                        <ModelCombobox
                          value={selectedNode.data.model ?? ''}
                          onChange={(model) => updateSelectedNodeData({ model })}
                          models={modelNames}
                        />
                      </label>

                      <label>
                        System prompt
                        <div className="template-field-row">
                          <LineNumberedTextarea
                            ref={systemPromptRef}
                            rows={5}
                            placeholder="You are a helpful assistant…"
                            value={selectedNode.data.systemPrompt ?? ''}
                            onChange={(next) => updateSelectedNodeData({ systemPrompt: next })}
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
                          <LineNumberedTextarea
                            ref={promptTemplateRef}
                            rows={4}
                            placeholder="e.g. please solve user problem: {{Input}}"
                            value={selectedNode.data.promptTemplate ?? ''}
                            onChange={(next) => updateSelectedNodeData({ promptTemplate: next })}
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
                        <SchemaBuilder
                          key={selectedNode.id}
                          value={selectedNode.data.outputSchema ?? ''}
                          onChange={(next) => updateSelectedNodeData({ outputSchema: next })}
                        />
                        <span className="field-hint">
                          If set, the reply must validate against this JSON Schema. On a mismatch the node
                          routes to its <code>fail</code> handle if you've wired one (e.g. back into a retry
                          loop); otherwise the turn fails. On success, downstream nodes can reference {'{{'}
                          {selectedNode.data.name || 'ThisNode'}.property{'}}'}.
                        </span>
                      </label>
                    </>
                  )}

                  {selectedNode.type === 'condition' && (
                    <>
                      <label>
                        Check
                        <select
                          value={selectedNode.data.conditionType ?? 'contains'}
                          onChange={(e) => {
                            const conditionType = e.target.value as NonNullable<AgentNodeData['conditionType']>
                            updateSelectedNodeData(
                              conditionType === 'similarity'
                                ? { conditionType, conditionThreshold: selectedNode.data.conditionThreshold ?? 0.85 }
                                : { conditionType },
                            )
                          }}
                        >
                          <option value="contains">contains</option>
                          <option value="not_contains">does not contain</option>
                          <option value="regex">matches regex</option>
                          <option value="json_schema">matches JSON schema</option>
                          <option value="similarity">similar to</option>
                        </select>
                        <span className="field-hint">
                          Routes to the <code>pass</code> handle when the check passes, <code>fail</code> otherwise.
                        </span>
                      </label>

                      {selectedNode.data.conditionType === 'json_schema' ? (
                        <label>
                          JSON schema
                          <SchemaBuilder
                            key={selectedNode.id}
                            value={selectedNode.data.conditionValue ?? ''}
                            onChange={(next) => updateSelectedNodeData({ conditionValue: next })}
                          />
                        </label>
                      ) : (
                        <label>
                          {selectedNode.data.conditionType === 'regex'
                            ? 'Pattern'
                            : selectedNode.data.conditionType === 'similarity'
                              ? 'Reference text'
                              : 'Value'}
                          <input
                            type="text"
                            placeholder={selectedNode.data.conditionType === 'regex' ? '\\bGOOD\\b' : 'e.g. GOOD'}
                            value={selectedNode.data.conditionValue ?? ''}
                            onChange={(e) => updateSelectedNodeData({ conditionValue: e.target.value })}
                          />
                        </label>
                      )}

                      {selectedNode.data.conditionType === 'similarity' && (
                        <label>
                          Threshold
                          <input
                            type="number"
                            min={0.05}
                            max={1}
                            step={0.05}
                            value={selectedNode.data.conditionThreshold ?? 0.85}
                            onChange={(e) =>
                              updateSelectedNodeData({ conditionThreshold: Number(e.target.value) || 0.85 })
                            }
                          />
                          <span className="field-hint">Passes when similarity ≥ this ratio (0–1).</span>
                        </label>
                      )}

                      <label>
                        Match against (optional)
                        <div className="template-field-row">
                          <LineNumberedTextarea
                            ref={matchTemplateRef}
                            rows={3}
                            placeholder="Leave blank to check the previous node's raw output"
                            value={selectedNode.data.matchTemplate ?? ''}
                            onChange={(next) => updateSelectedNodeData({ matchTemplate: next })}
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
                          Reference a specific node's output or JSON property instead — e.g. {'{{Judge.verdict}}'}.
                        </span>
                      </label>
                    </>
                  )}

                  {selectedNode.type === 'switch' && (
                    <>
                      <p className="field-hint">
                        The N-way sibling of a condition. The first case whose text appears (case-insensitive
                        substring) in the match text below wins and the walk follows that handle; no match takes the{' '}
                        <code>default</code> handle.
                      </p>

                      <div className="tool-arg-list">
                        <div className="inspector-section-label">Cases</div>
                        {(selectedNode.data.switchCases ?? []).map((c, i) => (
                          <div className="template-field-row" key={i}>
                            <input
                              type="text"
                              placeholder={`case ${i + 1} (e.g. billing)`}
                              value={c.value}
                              onChange={(e) => {
                                const next = [...(selectedNode.data.switchCases ?? [])]
                                next[i] = { value: e.target.value }
                                updateSelectedNodeData({ switchCases: next })
                              }}
                            />
                            <IconButton
                              icon={<X size={14} />}
                              label="Remove case"
                              onClick={() =>
                                updateSelectedNodeData({
                                  switchCases: (selectedNode.data.switchCases ?? []).filter((_, j) => j !== i),
                                })
                              }
                            />
                          </div>
                        ))}
                        <button
                          type="button"
                          className="link-button"
                          onClick={() =>
                            updateSelectedNodeData({ switchCases: [...(selectedNode.data.switchCases ?? []), { value: '' }] })
                          }
                        >
                          <Plus size={13} /> Add case
                        </button>
                      </div>

                      <label>
                        Match against (optional)
                        <div className="template-field-row">
                          <LineNumberedTextarea
                            ref={matchTemplateRef}
                            rows={3}
                            placeholder="Leave blank to check the previous node's raw output"
                            value={selectedNode.data.matchTemplate ?? ''}
                            onChange={(next) => updateSelectedNodeData({ matchTemplate: next })}
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
                          Reference a specific node's output or JSON property — e.g. {'{{Classifier.category}}'}.
                        </span>
                      </label>
                    </>
                  )}

                  {selectedNode.type === 'loop_start' && (
                    <label>
                      Max iterations
                      <input
                        type="number"
                        min={1}
                        value={selectedNode.data.loopMaxIterations ?? 5}
                        onChange={(e) => updateSelectedNodeData({ loopMaxIterations: Number(e.target.value) || 1 })}
                      />
                      <span className="field-hint">
                        Wire the <code>body</code> handle into the loop. Route a branch to a <b>Loop end</b> to keep
                        looping, or anywhere else to break out. After this many passes the <code>done</code> handle
                        fires. Reference the current pass downstream as {'{{'}
                        {selectedNode.data.name || 'Loop start'}.iteration{'}}'}.
                      </span>
                    </label>
                  )}

                  {selectedNode.type === 'loop_end' && (
                    <label>
                      Loops back to
                      <select
                        value={selectedNode.data.loopStartName ?? ''}
                        onChange={(e) => updateSelectedNodeData({ loopStartName: e.target.value })}
                      >
                        <option value="">Select a loop start…</option>
                        {nodes
                          .filter((n) => n.type === 'loop_start' && n.data.name)
                          .map((n) => (
                            <option key={n.id} value={n.data.name}>
                              {n.data.name}
                            </option>
                          ))}
                      </select>
                      <span className="field-hint">
                        Reaching this node jumps back to that Loop start for the next iteration (shown as the dashed
                        edge). Several branches can point at one Loop end.
                      </span>
                      {nodes.filter((n) => n.type === 'loop_start').length === 0 && (
                        <span className="error">No Loop start nodes in this graph — add one first.</span>
                      )}
                    </label>
                  )}

                  {selectedNode.type === 'state' && (
                    <>
                      <label>
                        Operation
                        <select
                          value={selectedNode.data.stateOp ?? 'append'}
                          onChange={(e) => updateSelectedNodeData({ stateOp: e.target.value as 'set' | 'append' })}
                        >
                          <option value="set">Set (replace)</option>
                          <option value="append">Append (newline-joined)</option>
                        </select>
                      </label>

                      <label>
                        Value (optional)
                        <div className="template-field-row">
                          <LineNumberedTextarea
                            ref={stateValueRef}
                            rows={3}
                            placeholder="Leave blank to use the previous node's output"
                            value={selectedNode.data.stateValue ?? ''}
                            onChange={(next) => updateSelectedNodeData({ stateValue: next })}
                          />
                          <VariableMenuButton
                            options={upstreamOptions}
                            onInsert={(snippet) =>
                              insertAtCursor(stateValueRef.current, selectedNode.data.stateValue ?? '', snippet, (next) =>
                                updateSelectedNodeData({ stateValue: next }),
                              )
                            }
                          />
                        </div>
                        <span className="field-hint">
                          The accumulated value is this node's output — reference it downstream as {'{{'}
                          {selectedNode.data.name || 'State'}
                          {'}}'}. Resets at the start of each chat turn.
                        </span>
                      </label>
                    </>
                  )}

                  {selectedNode.type === 'say' && (
                    <>
                      <p className="field-hint">
                        Sends a message to the user's chat as the agent works — for progress narration
                        ("searching the codebase…", "running tests…") ahead of the final answer.
                      </p>

                      <label>
                        Message
                        <div className="template-field-row">
                          <LineNumberedTextarea
                            ref={sayTemplateRef}
                            rows={3}
                            placeholder="e.g. Looking into {{Input}}…"
                            value={selectedNode.data.sayTemplate ?? ''}
                            onChange={(next) => updateSelectedNodeData({ sayTemplate: next })}
                          />
                          <VariableMenuButton
                            options={upstreamOptions}
                            onInsert={(snippet) =>
                              insertAtCursor(sayTemplateRef.current, selectedNode.data.sayTemplate ?? '', snippet, (next) =>
                                updateSelectedNodeData({ sayTemplate: next }),
                              )
                            }
                          />
                        </div>
                        <span className="field-hint">
                          Leave blank to repeat the previous node's output. The rendered text also flows on, so
                          downstream nodes can reference {'{{'}
                          {selectedNode.data.name || 'Say'}
                          {'}}'}.
                        </span>
                      </label>

                      <label className="checkbox-label">
                        <input
                          type="checkbox"
                          checked={selectedNode.data.sayFinal ?? false}
                          onChange={(e) => updateSelectedNodeData({ sayFinal: e.target.checked })}
                        />
                        This is the final answer
                      </label>
                      <span className="field-hint">
                        {selectedNode.data.sayFinal
                          ? 'This message is the turn’s definitive reply — shown prominently, not as dimmed progress.'
                          : 'A progress update — shown dimmed and collapsible once the final answer arrives. The turn’s reply is the last node’s output, or the last Say marked final.'}
                      </span>
                    </>
                  )}

                  {selectedNode.type === 'tool' && (
                    <>
                      <p className="field-hint">
                        Runs <b>one deterministic call</b> to the selected tool with the arguments below, then passes
                        its output on — no model decides anything here. For a model-driven tool-use loop, use an{' '}
                        <b>Agent</b> node instead.
                      </p>

                      {agentTools.length === 0 && (
                        <p className="hint">
                          This agent has no tools —{' '}
                          <button type="button" className="link-button" onClick={() => setSettingsOpen(true)}>
                            add some in Agent settings
                          </button>{' '}
                          to give it something to run.
                        </p>
                      )}

                      {agentTools.length > 0 && (
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

                      {agentTools.length > 0 && selectedNode.data.toolName && !selectedTool && (
                        <p className="error">
                          Tool "{selectedNode.data.toolName}" isn't in this agent's tools anymore — pick another.
                        </p>
                      )}

                      {!workspace && (
                        <p className="hint">
                          A Tool node also needs the agent bound to a workspace (Agent settings) — that's the
                          sandbox the command runs in.
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

                  {selectedNode.type === 'agent' && (
                    <>
                      <label>
                        Model
                        <ModelCombobox
                          value={selectedNode.data.agentModel ?? ''}
                          onChange={(agentModel) => updateSelectedNodeData({ agentModel })}
                          models={modelNames}
                        />
                      </label>

                      <label>
                        Instructions
                        <div className="template-field-row">
                          <LineNumberedTextarea
                            ref={agentInstructionsRef}
                            rows={5}
                            placeholder="You are a research assistant. Use the tools to answer the user's question…"
                            value={selectedNode.data.agentInstructions ?? ''}
                            onChange={(next) => updateSelectedNodeData({ agentInstructions: next })}
                          />
                          <VariableMenuButton
                            options={upstreamOptions}
                            onInsert={(snippet) =>
                              insertAtCursor(
                                agentInstructionsRef.current,
                                selectedNode.data.agentInstructions ?? '',
                                snippet,
                                (next) => updateSelectedNodeData({ agentInstructions: next }),
                              )
                            }
                          />
                        </div>
                        <span className="field-hint">
                          The whole task and persona for this node — the model loops on it, emitting{' '}
                          <code>ACTION</code>/<code>ARGS</code> to call a tool below or <code>FINAL</code> to answer.
                          Leave blank to use the previous node's output as the task.
                        </span>
                      </label>

                      <label>
                        Max iterations
                        <input
                          type="number"
                          min={1}
                          value={selectedNode.data.agentMaxIterations ?? 6}
                          onChange={(e) => updateSelectedNodeData({ agentMaxIterations: Number(e.target.value) || 1 })}
                        />
                      </label>

                      <p className="field-hint">
                        This node runs a <b>model-driven loop</b> — the model repeatedly picks a tool or knowledge
                        search, reads the result, and decides what to do next, bounded by the max iterations above.
                        For a single fixed tool call with no model in the loop, use a <b>Tool</b> node instead.
                      </p>

                      <div className="tool-arg-list">
                        <div className="inspector-section-label">Tools it can call</div>
                        {agentTools.length === 0 ? (
                          <p className="hint">
                            This agent has no tools —{' '}
                            <button type="button" className="link-button" onClick={() => setSettingsOpen(true)}>
                              add some in Agent settings
                            </button>
                            .
                          </p>
                        ) : (
                          <MultiPickList
                            options={availableTools}
                            selected={selectedNode.data.agentTools ?? []}
                            onToggle={(toolName, checked) => {
                              const cur = selectedNode.data.agentTools ?? []
                              updateSelectedNodeData({
                                agentTools: checked ? [...cur, toolName] : cur.filter((n) => n !== toolName),
                              })
                            }}
                            emptyMessage="The agent's tool set is empty."
                          />
                        )}

                        <div className="inspector-section-label">Knowledge bases it can search</div>
                        <MultiPickList
                          options={availableKnowledgeBases}
                          selected={selectedNode.data.agentKnowledgeBases ?? []}
                          onToggle={(kbName, checked) => {
                            const cur = selectedNode.data.agentKnowledgeBases ?? []
                            updateSelectedNodeData({
                              agentKnowledgeBases: checked ? [...cur, kbName] : cur.filter((n) => n !== kbName),
                            })
                          }}
                          emptyMessage="The agent has no knowledge bases — add some in Agent settings."
                        />
                        <span className="field-hint">
                          Offered to the model as a built-in <code>knowledge_search</code> action — a deterministic
                          keyword lookup, and it needs no workspace.
                        </span>
                      </div>

                      <label>
                        Output schema (optional)
                        <SchemaBuilder
                          key={selectedNode.id}
                          value={selectedNode.data.agentOutputSchema ?? ''}
                          onChange={(next) => updateSelectedNodeData({ agentOutputSchema: next })}
                        />
                        <span className="field-hint">
                          If set, the FINAL answer must validate against this JSON Schema. On a mismatch the node
                          routes to its <code>fail</code> handle if wired, else the turn fails; on success,
                          downstream nodes can reference {'{{'}
                          {selectedNode.data.name || 'ThisNode'}.property{'}}'}.
                        </span>
                      </label>
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
                          {availableKnowledgeBases.map((kb) => (
                            <option key={kb.name} value={kb.name}>
                              {kb.name}
                            </option>
                          ))}
                        </select>
                        {agentKnowledgeBases.length === 0 && (
                          <span className="field-hint">
                            This agent has no knowledge bases —{' '}
                            <button type="button" className="link-button" onClick={() => setSettingsOpen(true)}>
                              add some in Agent settings
                            </button>
                            .
                          </span>
                        )}
                      </label>

                      <label>
                        Query (optional)
                        <div className="template-field-row">
                          <LineNumberedTextarea
                            ref={knowledgeQueryRef}
                            rows={3}
                            placeholder="Leave blank to query with the previous node's raw output"
                            value={selectedNode.data.knowledgeQuery ?? ''}
                            onChange={(next) => updateSelectedNodeData({ knowledgeQuery: next })}
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
                          Matching records are joined and passed downstream as this node's output — a
                          deterministic keyword match against title and content (all query words must appear), not
                          embeddings.
                        </span>
                      </label>

                      <label>
                        Max results
                        <input
                          type="number"
                          min={0}
                          value={selectedNode.data.knowledgeMaxResults ?? 0}
                          onChange={(e) =>
                            updateSelectedNodeData({ knowledgeMaxResults: Math.max(0, Math.floor(Number(e.target.value) || 0)) })
                          }
                        />
                        <span className="field-hint">
                          Keep only the top N matches (in the knowledge base's own record order). 0 = no limit.
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
          workspace={workspace}
          testWorkspaces={workspaces.filter((w) => w.type === 'test')}
          toolCatalog={toolCatalog}
          knowledgeBases={knowledgeBases}
          agentTools={agentTools}
          agentKnowledgeBases={agentKnowledgeBases}
          description={description}
          onChangeWorkspace={setWorkspace}
          onToggleTool={(n, checked) => setAgentTools((cur) => (checked ? [...cur, n] : cur.filter((x) => x !== n)))}
          onToggleKnowledgeBase={(n, checked) =>
            setAgentKnowledgeBases((cur) => (checked ? [...cur, n] : cur.filter((x) => x !== n)))
          }
          onChangeDescription={setDescription}
          onClose={() => setSettingsOpen(false)}
        />
      )}

      {chatOpen && (
        <Modal title={`Chat with ${name}`} onClose={closeChat}>
          <label className="run-workspace-picker">
            Test workspace
            <select
              value={runWorkspace}
              onChange={(e) => {
                setRunWorkspace(e.target.value)
                if (runId) void stopAgentRun(runId).catch(() => {})
                setRunId(null)
                setMessages([])
                setSteps([])
                void handleStartRun()
              }}
            >
              <option value="">{workspace ? `Agent default (${workspace})` : 'None'}</option>
              {workspaces
                .filter((w) => w.type === 'test')
                .map((w) => (
                  <option key={w.name} value={w.name}>
                    {w.name}
                  </option>
                ))}
            </select>
            <span className="field-hint">
              A fresh copy is staged per run — find its folder in Workspaces → Running sandboxes to watch the agent work.
            </span>
          </label>
          {!runId && <p className="hint">Starting a run…</p>}
          {runId && (
            <>
              <div className="chat-log">
                {messages.length === 0 && <p className="hint">Say hello to try your agent.</p>}
                {renderChat(messages)}
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
                  <ul className="event-log agent-debug-feed">
                    {steps.map((s, i) => (
                      <li
                        key={i}
                        className={
                          s.phase === 'start'
                            ? 'agent-debug-feed-start'
                            : s.phase === 'tool'
                              ? 'agent-debug-feed-tool'
                              : undefined
                        }
                      >
                        <span className="event-type">{s.phase === 'tool' ? 'tool call' : s.nodeType}</span>
                        <span className="event-data">
                          {s.phase === 'tool' && s.command ? (
                            <code className="agent-tool-call">{s.command}</code>
                          ) : (
                            <Expandable text={s.output} limit={200} />
                          )}
                        </span>
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

// renderChat walks the chat entries and renders normal user/assistant
// bubbles, grouping consecutive "progress" (say-node narration) entries into
// one collapsible block that auto-collapses once a later message (the final
// answer) follows it.
function renderChat(entries: ChatEntry[]) {
  const out: React.ReactNode[] = []
  let i = 0
  while (i < entries.length) {
    if (entries[i].kind === 'progress') {
      const group: ChatEntry[] = []
      while (i < entries.length && entries[i].kind === 'progress') {
        group.push(entries[i])
        i++
      }
      out.push(<ProgressGroup key={`progress-${i}`} items={group} settled={i < entries.length} />)
      continue
    }
    const m = entries[i]
    out.push(
      <div key={i} className={`chat-message chat-message-${m.role}`}>
        <span className="chat-role">{m.role}</span>
        <span>{m.content}</span>
      </div>,
    )
    i++
  }
  return out
}

// ProgressGroup shows a run of say-node progress messages. It starts open
// (you watch it work) and collapses itself once `settled` becomes true — i.e.
// once the final answer has arrived — while still letting you expand it back.
function ProgressGroup({ items, settled }: { items: ChatEntry[]; settled: boolean }) {
  const [open, setOpen] = useState(!settled)
  useEffect(() => {
    if (settled) setOpen(false)
  }, [settled])
  return (
    <div className="chat-progress-group">
      <button type="button" className="chat-progress-toggle" onClick={() => setOpen((o) => !o)}>
        {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        {settled ? `${items.length} step${items.length === 1 ? '' : 's'}` : 'Working…'}
      </button>
      {open &&
        items.map((it, k) => (
          <div key={k} className="chat-message chat-message-progress">
            <span>{it.content}</span>
          </div>
        ))}
    </div>
  )
}

// DebugActivity is the live event feed for a debug session — moved out of the
// (cramped) left debug panel into the right sidebar. Each entry truncates long
// text with a "Show more" toggle; a phase="tool" event shows the raw rendered
// command the agent/tool node ran.
function DebugActivity({
  feed,
  lastStep,
  pending,
}: {
  feed: AgentStepEvent[]
  lastStep: DebugState['lastStep']
  pending: boolean
}) {
  if (feed.length === 0 && !lastStep) {
    return <p className="hint">Step through a turn and each node's activity shows up here.</p>
  }
  return (
    <div className="agent-activity-panel">
      {feed.length > 0 && (
        <>
          <div className="agent-palette-label">Activity{pending ? ' · running…' : ''}</div>
          <ul className="event-log agent-debug-feed">
            {feed.map((s, i) => {
              const cls =
                s.phase === 'start'
                  ? 'agent-debug-feed-start'
                  : s.phase === 'tool'
                    ? 'agent-debug-feed-tool'
                    : undefined
              return (
                <li key={i} className={cls}>
                  <span className="event-type">{s.phase === 'tool' ? 'tool call' : s.nodeType}</span>
                  <span className="event-data">
                    {s.phase === 'tool' && s.command ? (
                      <code className="agent-tool-call">{s.command}</code>
                    ) : (
                      <Expandable text={s.output} limit={200} />
                    )}
                  </span>
                </li>
              )
            })}
          </ul>
        </>
      )}

      {lastStep && (
        <>
          <div className="agent-palette-label">Last step — {lastStep.nodeType}</div>
          <Expandable text={lastStep.output || '(empty output)'} limit={400} pre className="agent-debug-output" />
        </>
      )}
    </div>
  )
}

interface AgentSettingsModalProps {
  workspace: string
  testWorkspaces: Workspace[]
  toolCatalog: Tool[]
  knowledgeBases: KnowledgeBase[]
  agentTools: string[]
  agentKnowledgeBases: string[]
  description: string
  onChangeWorkspace: (value: string) => void
  onToggleTool: (name: string, checked: boolean) => void
  onToggleKnowledgeBase: (name: string, checked: boolean) => void
  onChangeDescription: (value: string) => void
  onClose: () => void
}

function AgentSettingsModal({
  workspace,
  testWorkspaces,
  toolCatalog,
  knowledgeBases,
  agentTools,
  agentKnowledgeBases,
  description,
  onChangeWorkspace,
  onToggleTool,
  onToggleKnowledgeBase,
  onChangeDescription,
  onClose,
}: AgentSettingsModalProps) {
  return (
    <Modal title="Agent settings" onClose={onClose} size="lg">
      <div className="stacked-form">
        <label>
          Workspace
          <select value={workspace} onChange={(e) => onChangeWorkspace(e.target.value)}>
            <option value="">None</option>
            {testWorkspaces.map((w) => (
              <option key={w.name} value={w.name}>
                {w.name}
              </option>
            ))}
          </select>
          <span className="field-hint">
            A TEST workspace — a fresh copy of its files is the sandbox this agent's Tool/Agent nodes act
            in. Real workspaces aren't selectable here; those are used through Deployments.
          </span>
        </label>

        <div className="inspector-section-label">Tools</div>
        <MultiPickList
          options={toolCatalog}
          selected={agentTools}
          onToggle={onToggleTool}
          emptyMessage="No tools in the catalog yet — create some on the Tools page."
        />

        <div className="inspector-section-label">Knowledge bases</div>
        <MultiPickList
          options={knowledgeBases}
          selected={agentKnowledgeBases}
          onToggle={onToggleKnowledgeBase}
          emptyMessage="No knowledge bases yet — create one on the Knowledge page."
        />

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
