export type ModelSource = 'mlx' | 'binary' | string

export interface Model {
  name: string
  baseModel?: string
  source: ModelSource
}

export interface ModelDetail {
  name: string
  baseModel?: string
  source: ModelSource
  trainingRun?: TrainingRun
}

export type TensorBlock = 'embedding' | 'layer' | 'norm' | 'lm_head' | 'other'

export interface TensorSummary {
  name: string
  dtype: string
  shape: number[]
  numElements: number
  block: TensorBlock
  layer?: number
}

export interface Architecture {
  modelType?: string
  numLayers: number
  hiddenSize?: number
  vocabSize?: number
  numParameters: number
  estimatedBytes: number
  tensors: TensorSummary[]
}

export interface HeatmapData {
  rows: number
  cols: number
  grid: number[]
  min: number
  max: number
  mean: number
  std: number
}

export interface TokenProbability {
  token: string
  logprob: number
}

export interface TokenPosition {
  token: string
  logprob: number
  topCandidates: TokenProbability[]
}

export interface Example {
  input: string
  output: string
  description?: string
  tags?: string[]
}

export interface DatasetSummary {
  name: string
  title?: string
  description?: string
  createdAt: string
  pairCount: number
}

export interface DatasetDetail {
  name: string
  title?: string
  description?: string
  examples: Example[]
}

export interface TrainingConfig {
  baseModel: string
  dataset: string
  outputName: string
  iterations: number
  learningRate?: number
}

export type TrainingStatus = 'running' | 'succeeded' | 'failed' | 'cancelled'

export interface ProgressPoint {
  timestamp: string
  iteration: number
  trainLoss?: number
  valLoss?: number
  peakMemGB?: number
  tokensPerSec?: number
}

export interface TrainingRun {
  id: string
  config: TrainingConfig
  status: TrainingStatus
  startedAt: string
  finishedAt?: string
  progress: ProgressPoint[]
  error?: string
}

export interface Mount {
  hostPath: string
  containerPath: string
  readOnly?: boolean
}

export type ToolParameterType = 'string' | 'number' | 'boolean'

export interface ToolParameter {
  name: string
  type: ToolParameterType
  description?: string
  required: boolean
}

// Tool is a global catalog entry (its own top-level resource, like Models/
// Datasets) — an Environment names which catalog tools it makes available
// (Environment.tools: string[]) rather than embedding a copy, so editing a
// tool here changes it everywhere it's attached.
export interface Tool {
  name: string
  description?: string
  command: string
  parameters: ToolParameter[]
  prebuilt?: boolean
  createdAt: string
}

export interface Environment {
  name: string
  image: string
  tools: string[]
  mounts: Mount[]
  prebuilt: boolean
  createdAt: string
}

export interface KnowledgeRecord {
  id: string
  title: string
  content: string
  tags?: string[]
}

// KnowledgeBase is independent of any Environment — querying its records is
// plain in-process text matching, nothing to launch. See internal/knowledge:
// querying is deterministic keyword/substring matching, not embeddings.
export interface KnowledgeBase {
  name: string
  description?: string
  records: KnowledgeRecord[]
  createdAt: string
}

export interface Instance {
  id: string
  name: string
  environmentName: string
  image: string
  state: string
  createdAt: string
}

export type ExecStatus = 'running' | 'done' | 'failed'

export interface Exec {
  id: string
  instanceId: string
  command: string
  output: string
  exitCode?: number
  status: ExecStatus
  startedAt: string
  finishedAt?: string
  error?: string
}

// There's no "output" node type — any node with no outgoing edge is simply
// where a turn ends, and its own output becomes the reply (see
// internal/agents.Engine). Edges may form cycles: a loop or condition node
// routing back to an earlier node is how plan-execute-judge, Ralph loops,
// and similar architectures are built.
export type NodeType =
  | 'input'
  | 'prompt'
  | 'tool'
  | 'knowledge'
  | 'condition'
  | 'switch'
  | 'loop_start'
  | 'loop_end'
  | 'state'
  | 'say'
  | 'agent'

export interface SwitchCase {
  value: string
}

export interface AgentNodeData extends Record<string, unknown> {
  // name is a stable, user-editable, unique-within-the-graph display name —
  // shown on the canvas and used as the token a downstream node's template
  // references: {{name}} for this node's raw text output, or
  // {{name.property}} for a property of it if outputSchema made that output
  // parseable JSON. A node with no name isn't referenceable at all.
  name?: string

  // debugHighlight is set locally by AgentEditor while a debug session is
  // active (see DebugState below) to outline the pending/most-recently-
  // executed node on the canvas — purely a rendering flag, never sent to
  // or read from the backend.
  debugHighlight?: 'pending' | 'executed'

  model?: string
  systemPrompt?: string
  // promptTemplate is templated user-turn text ({{nodeName}}/{{nodeName.field}}
  // resolved against every earlier-run node); falls back to the previous
  // node's raw output verbatim when empty.
  promptTemplate?: string
  // outputSchema, if set, is a JSON Schema the reply must validate against —
  // failing validation fails the whole turn. Enables downstream
  // {{thisNode.property}} references.
  outputSchema?: string

  // Condition nodes: a single deterministic check (the fields of a
  // registry.Assertion) run via internal/assertions against matchTemplate's
  // rendered text, routing to the "pass" or "fail" handle. matchTemplate
  // falls back to the inbound value when empty. Generalizes the former
  // keyword-only "decision" node.
  conditionType?: 'contains' | 'not_contains' | 'regex' | 'json_schema' | 'similarity'
  conditionValue?: string
  conditionThreshold?: number
  matchTemplate?: string

  // Switch nodes: the N-way sibling of condition. switchCases is an ordered
  // list — the first whose value is a case-insensitive substring of the
  // (templated) matchTemplate text wins, and the walk follows the outgoing
  // handle named by that value; no match takes the "default" handle.
  switchCases?: SwitchCase[]

  // loop_start / loop_end are a matched pair. loop_start: loopMaxIterations
  // caps how many times the walk may enter it before routing to "done"
  // instead of "body"; {{thisNode.iteration}} is available downstream.
  // loop_end: loopStartName names the loop_start it jumps back to (the
  // back-edge is implicit). Any number of branches can converge on a
  // loop_end to "continue"; a branch wired elsewhere "breaks" out.
  loopMaxIterations?: number
  loopStartName?: string

  // State nodes: stateOp is "set" or "append"; stateValue is the templated
  // value (falls back to the inbound value when empty). The accumulator
  // lives under the node's own name, so {{thisNode}} reads its current value.
  stateOp?: 'set' | 'append'
  stateValue?: string

  // Tool nodes: toolName names a Tool declared on the agent's bound
  // Environment (see registry.Tool); each toolArgs value may itself contain
  // {{nodeName}}/{{nodeName.field}} template references.
  toolName?: string
  toolArgs?: Record<string, string>

  // Say nodes: emit a user-facing message mid-turn. sayTemplate is templated
  // text (falls back to the inbound value when empty); sayFinal marks it as
  // the turn's definitive reply rather than a progress update — the last
  // final say message emitted wins, else the terminal node's own output is
  // the reply. Progress messages stream to the chat live but aren't added to
  // conversation history.
  sayTemplate?: string
  sayFinal?: boolean

  // Agent nodes: a bounded LLM tool-calling loop. agentInstructions is the
  // templated goal/system text; agentTools is a subset of the bound
  // Environment's tool names the loop may call; agentKnowledgeBases names
  // KnowledgeBases the loop may search via a built-in "knowledge_search"
  // pseudo-tool (needs no Environment); agentMaxIterations caps the internal
  // loop.
  agentInstructions?: string
  agentModel?: string
  agentMaxIterations?: number
  agentTools?: string[]
  agentKnowledgeBases?: string[]
  // If set, a JSON Schema the FINAL answer must validate against. On a
  // mismatch the node routes to its "fail" handle if wired, else the turn
  // fails; on success {{thisNode.property}} is available downstream.
  agentOutputSchema?: string

  // Knowledge nodes: knowledgeBaseName names a KnowledgeBase (independent of
  // any Environment) to search; knowledgeQuery is templated query text,
  // falling back to the previous node's raw output when empty, same
  // convention as promptTemplate/matchTemplate. knowledgeMaxResults caps how
  // many matching records flow downstream (0 / unset = all).
  knowledgeBaseName?: string
  knowledgeQuery?: string
  knowledgeMaxResults?: number
}

export interface AgentNode {
  id: string
  type: NodeType
  position: { x: number; y: number }
  data: AgentNodeData
}

export interface AgentEdge {
  id: string
  source: string
  sourceHandle?: string
  target: string
}

export interface AgentGraph {
  nodes: AgentNode[]
  edges: AgentEdge[]
}

export interface Agent {
  name: string
  environment?: string
  description?: string
  graph: AgentGraph
  createdAt: string
}

export type ChatRole = 'user' | 'assistant'

export interface ChatMessage {
  role: ChatRole
  content: string
  timestamp: string
}

export interface AgentRun {
  id: string
  agentName: string
  instanceId?: string
  messages: ChatMessage[]
  createdAt: string
}

export interface AgentStepEvent {
  runId: string
  nodeId: string
  nodeType: string
  output: string
  // "start" = the node just began long-running work (an LLM call); output is
  // a short status like "calling model X". Absent for a normal result event.
  phase?: 'start'
}

// AgentMessageEvent is a user-facing message a "say" node emitted mid-turn,
// streamed on the "agent.message" SSE channel: a progress update shown live
// as the agent works, or the turn's definitive reply.
export interface AgentMessageEvent {
  runId: string
  kind: 'progress' | 'final'
  content: string
}

export interface AgentStepResult {
  nodeId: string
  nodeType: string
  output: string
}

// DebugState is a paused, step-by-step chat session's current snapshot —
// Debug's counterpart to AgentRun. pendingNodeId/pendingNodeType name the
// node the next "step" call will execute (unset once finished, or before
// the first message is sent); lastStep is the most recently executed
// node's own result (undefined until at least one step has run) — the
// same one "retry" would redo.
export interface DebugState {
  id: string
  agentName: string
  instanceId?: string
  messages: ChatMessage[]
  pendingNodeId?: string
  pendingNodeType?: string
  lastStep?: AgentStepResult
  finished: boolean
  createdAt: string
}

export type AssertionType = 'contains' | 'not_contains' | 'regex' | 'json_schema' | 'similarity'

export interface Assertion {
  type: AssertionType
  value: string
  // threshold is only meaningful for "similarity" (the minimum required
  // similarity ratio, in (0, 1]).
  threshold?: number
}

// VerifyStep is a shell command run in a test case's launched Environment
// instance after the agent's turn finishes, checked with the same
// assertion types used against a reply — Evaluations only. The command's
// exit code is deliberately not itself pass/fail; only its assertions are.
export interface VerifyStep {
  command: string
  assertions: Assertion[]
}

export interface TestCase {
  id: string
  prompt: string
  // setup and verifyCommands are Evaluations-only (a Benchmark's test
  // cases leave them unset — there's no Environment to prepare or verify):
  // setup runs in sequence in the test case's launched instance before the
  // agent's turn; verifyCommands run after, checking the environment's
  // resulting state.
  setup?: string[]
  assertions: Assertion[]
  verifyCommands?: VerifyStep[]
  // tags are only used by Benchmarks/Evaluations, for filtering the test
  // case list — same role as Example.tags for datasets.
  tags?: string[]
}

export type EvalRunStatus = 'running' | 'succeeded' | 'failed'

export interface AssertionResult extends Assertion {
  passed: boolean
  error?: string
}

export interface VerifyStepResult {
  command: string
  output: string
  assertions: AssertionResult[]
  passed: boolean
  error?: string
}

export interface TestCaseResult {
  testCaseId: string
  prompt: string
  instanceId?: string
  reply: string
  assertions: AssertionResult[]
  verifyResults?: VerifyStepResult[]
  passed: boolean
  error?: string
}

// Evaluation mirrors Benchmark's draft/published-version split exactly
// (see Benchmark below) — Environment is the one difference: a live
// setting (like an Agent's own Environment binding), not versioned
// content, since it's not part of "what does this test suite check." For
// a test case's setup/verifyCommands to mean anything, the agent(s) this
// evaluation runs against should themselves be bound to this same
// Environment — setup/verify run in the exact instance the agent's own
// Tool nodes act in during its turn, not a second, separate container.
export interface Evaluation {
  name: string
  environment?: string
  version: number
  testCases: TestCase[]
  createdAt: string
}

// EvaluationVersion is an immutable snapshot of an evaluation's test
// cases, created by publishEvaluationVersion. Once published, a version's
// testCases never change.
export interface EvaluationVersion {
  version: number
  testCases: TestCase[]
  publishedAt: string
}

// EvaluationRunResult is one agent's durable outcome for a specific
// version of an evaluation's test cases — running the same evaluation
// version against the same agent again overwrites its previous result.
// This is the persisted, queryable comparison data the evaluation detail
// page's "run results" view sorts and lists; EvaluationRun below is only
// the ephemeral in-progress/just-finished execution that produced it.
export interface EvaluationRunResult {
  evaluationVersion: number
  agentName: string
  results: TestCaseResult[]
  passed: number
  total: number
  startedAt: string
  finishedAt: string
  error?: string
}

export interface EvaluationRun {
  id: string
  evaluationName: string
  evaluationVersion: number
  agentNames: string[]
  environmentName?: string
  status: EvalRunStatus
  results: EvaluationRunResult[]
  startedAt: string
  finishedAt?: string
  error?: string
}

export interface EvaluationProgressEvent {
  runId: string
  agentName: string
  testCaseId: string
  passed: boolean
}

export interface Benchmark {
  name: string
  // version is the number of the most recently PUBLISHED version (0 = none
  // published yet) — it does not track draft edits. testCases below is the
  // live, freely-editable draft; publishing (see publishBenchmarkVersion)
  // snapshots it into an immutable BenchmarkVersion, which is the only
  // thing a run can actually target.
  version: number
  testCases: TestCase[]
  createdAt: string
}

// BenchmarkVersion is an immutable snapshot of a benchmark's test cases,
// created by publishBenchmarkVersion. Once published, a version's testCases
// never change.
export interface BenchmarkVersion {
  version: number
  testCases: TestCase[]
  publishedAt: string
}

// BenchmarkRunResult is one model's durable outcome for a specific version
// of a benchmark's test cases — running the same benchmark version against
// the same model again overwrites its previous result. This is the
// persisted, queryable comparison data the benchmark detail page's "run
// results" view sorts and lists; BenchmarkRun below is only the ephemeral
// in-progress/just-finished execution that produced it.
export interface BenchmarkRunResult {
  benchmarkVersion: number
  modelName: string
  results: TestCaseResult[]
  passed: number
  total: number
  startedAt: string
  finishedAt: string
  error?: string
}

export interface BenchmarkRun {
  id: string
  benchmarkName: string
  benchmarkVersion: number
  modelNames: string[]
  status: EvalRunStatus
  results: BenchmarkRunResult[]
  startedAt: string
  finishedAt?: string
  error?: string
}

export interface BenchmarkProgressEvent {
  runId: string
  modelName: string
  testCaseId: string
  passed: boolean
}

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error ?? `request failed with status ${res.status}`)
  }
  return res.json() as Promise<T>
}

// noContent checks a 204-style response for errors without trying to parse
// a (likely empty) JSON body on success.
async function noContent(res: Response): Promise<void> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error ?? `request failed with status ${res.status}`)
  }
}

export function listModels(): Promise<Model[]> {
  return fetch('/api/models').then(json<Model[]>)
}

export function getModel(name: string): Promise<ModelDetail> {
  return fetch(`/api/models/${encodeURIComponent(name)}`).then(json<ModelDetail>)
}

export function deleteModel(name: string): Promise<void> {
  return fetch(`/api/models/${encodeURIComponent(name)}`, { method: 'DELETE' }).then(noContent)
}

// HuggingFaceModel is one result from GET /api/huggingface/models — an
// mlx-community model on the Hub. `added` is true when a local registry
// model already tracks this repo. `name` is the short name it would be
// registered under.
export interface HuggingFaceModel {
  repoId: string
  name: string
  downloads: number
  likes: number
  tags: string[]
  lastModified: string
  added: boolean
}

// searchHuggingFaceModels runs a user-initiated search of the mlx-community
// org on the Hugging Face Hub. An empty query returns the most downloaded.
export function searchHuggingFaceModels(query: string): Promise<HuggingFaceModel[]> {
  return fetch(`/api/huggingface/models?q=${encodeURIComponent(query)}`).then(json<HuggingFaceModel[]>)
}

// addHuggingFaceModel registers an mlx-community repo as a local model. No
// weights are fetched now — mlx_lm downloads them on first use.
export function addHuggingFaceModel(repoId: string): Promise<Model> {
  return fetch('/api/huggingface/models', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ repoId }),
  }).then(json<Model>)
}

export interface ChatTurn {
  role: 'user' | 'assistant'
  content: string
}

// chatWithModel sends the full conversation so far (including the newest
// user turn) — mlx_lm.server is stateless between requests, so nothing
// persists between calls on the backend either.
export function chatWithModel(name: string, messages: ChatTurn[]): Promise<{ completion: string }> {
  return fetch(`/api/models/${encodeURIComponent(name)}/chat`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ messages }),
  }).then(json<{ completion: string }>)
}

// getModelArchitecture reads a model's derived topology straight from its
// .safetensors header(s) on disk — no tensor weight bytes are transferred.
export function getModelArchitecture(name: string): Promise<Architecture> {
  return fetch(`/api/models/${encodeURIComponent(name)}/architecture`).then(json<Architecture>)
}

// getModelHeatmap fetches a single tensor's values, subsampled down to
// gridSize x gridSize (default matches the backend's own default) plus
// summary statistics over the full tensor.
export function getModelHeatmap(name: string, tensor: string, gridSize?: number): Promise<HeatmapData> {
  const params = new URLSearchParams({ tensor })
  if (gridSize) params.set('grid', String(gridSize))
  return fetch(`/api/models/${encodeURIComponent(name)}/heatmap?${params}`).then(json<HeatmapData>)
}

// getTokenProbabilities generates a short completion for prompt and returns,
// for each generated token, the top candidate tokens the model considered.
export function getTokenProbabilities(
  name: string,
  prompt: string,
  maxTokens?: number,
  topLogprobs?: number,
): Promise<{ positions: TokenPosition[] }> {
  return fetch(`/api/models/${encodeURIComponent(name)}/token-probabilities`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ prompt, maxTokens, topLogprobs }),
  }).then(json<{ positions: TokenPosition[] }>)
}

export function listDatasets(): Promise<DatasetSummary[]> {
  return fetch('/api/datasets').then(json<DatasetSummary[]>)
}

export function createDataset(name: string, title?: string, description?: string): Promise<DatasetSummary> {
  return fetch('/api/datasets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, title, description }),
  }).then(json<DatasetSummary>)
}

export function getDataset(name: string): Promise<DatasetDetail> {
  return fetch(`/api/datasets/${encodeURIComponent(name)}`).then(json<DatasetDetail>)
}

export function deleteDataset(name: string): Promise<void> {
  return fetch(`/api/datasets/${encodeURIComponent(name)}`, { method: 'DELETE' }).then(noContent)
}

export function generateVariations(
  name: string,
  req: { model: string; seed: Example; count: number },
): Promise<Example[]> {
  return fetch(`/api/datasets/${encodeURIComponent(name)}/variations`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  }).then(json<Example[]>)
}

export function addExamples(name: string, examples: Example[]): Promise<Example[]> {
  return fetch(`/api/datasets/${encodeURIComponent(name)}/examples`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ examples }),
  }).then(json<Example[]>)
}

export function updateExample(name: string, index: number, example: Example): Promise<Example> {
  return fetch(`/api/datasets/${encodeURIComponent(name)}/examples/${index}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(example),
  }).then(json<Example>)
}

export function deleteExample(name: string, index: number): Promise<void> {
  return fetch(`/api/datasets/${encodeURIComponent(name)}/examples/${index}`, { method: 'DELETE' }).then(noContent)
}

export function importDataset(name: string, format: 'csv' | 'jsonl', content: string): Promise<Example[]> {
  return fetch(`/api/datasets/${encodeURIComponent(name)}/import`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ format, content }),
  }).then(json<Example[]>)
}

export function datasetExportUrl(name: string, format: 'csv' | 'jsonl'): string {
  return `/api/datasets/${encodeURIComponent(name)}/export?format=${format}`
}

export function startTrainingRun(cfg: TrainingConfig): Promise<TrainingRun> {
  return fetch('/api/training/runs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
  }).then(json<TrainingRun>)
}

export function listTrainingRuns(): Promise<TrainingRun[]> {
  return fetch('/api/training/runs').then(json<TrainingRun[]>)
}

export function getTrainingRun(id: string): Promise<TrainingRun> {
  return fetch(`/api/training/runs/${encodeURIComponent(id)}`).then(json<TrainingRun>)
}

export function cancelTrainingRun(id: string): Promise<void> {
  return fetch(`/api/training/runs/${encodeURIComponent(id)}/cancel`, { method: 'POST' }).then(noContent)
}

export function listEnvironments(): Promise<Environment[]> {
  return fetch('/api/environments').then(json<Environment[]>)
}

export function createEnvironment(env: { name: string; image: string; mounts: Mount[] }): Promise<Environment> {
  return fetch('/api/environments', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(env),
  }).then(json<Environment>)
}

export function getEnvironment(name: string): Promise<Environment> {
  return fetch(`/api/environments/${encodeURIComponent(name)}`).then(json<Environment>)
}

export function deleteEnvironment(name: string): Promise<void> {
  return fetch(`/api/environments/${encodeURIComponent(name)}`, { method: 'DELETE' }).then(noContent)
}

export function updateEnvironmentConfig(name: string, image: string, mounts: Mount[]): Promise<Environment> {
  return fetch(`/api/environments/${encodeURIComponent(name)}/config`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ image, mounts }),
  }).then(json<Environment>)
}

// attachTool references an existing catalog tool from an environment — a
// live reference, not a copy: editing the tool afterward (via updateTool)
// changes it everywhere it's attached.
export function attachTool(name: string, toolName: string): Promise<Environment> {
  return fetch(`/api/environments/${encodeURIComponent(name)}/tools`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ toolName }),
  }).then(json<Environment>)
}

// detachTool removes a tool reference from an environment without touching
// the catalog entry itself.
export function detachTool(name: string, toolName: string): Promise<void> {
  return fetch(`/api/environments/${encodeURIComponent(name)}/tools/${encodeURIComponent(toolName)}`, { method: 'DELETE' }).then(
    noContent,
  )
}

export function tryTool(name: string, toolName: string, instanceId: string, args: Record<string, string>): Promise<Exec> {
  return fetch(`/api/environments/${encodeURIComponent(name)}/tools/${encodeURIComponent(toolName)}/try`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ instanceId, args }),
  }).then(json<Exec>)
}

// --- Tool catalog (global, independent of any Environment) ---

export function listTools(): Promise<Tool[]> {
  return fetch('/api/tools').then(json<Tool[]>)
}

export function createTool(tool: { name: string; description?: string; command: string; parameters: ToolParameter[] }): Promise<Tool> {
  return fetch('/api/tools', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(tool),
  }).then(json<Tool>)
}

export function getTool(name: string): Promise<Tool> {
  return fetch(`/api/tools/${encodeURIComponent(name)}`).then(json<Tool>)
}

export function updateTool(
  name: string,
  tool: { description?: string; command: string; parameters: ToolParameter[] },
): Promise<Tool> {
  return fetch(`/api/tools/${encodeURIComponent(name)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(tool),
  }).then(json<Tool>)
}

export function deleteTool(name: string): Promise<void> {
  return fetch(`/api/tools/${encodeURIComponent(name)}`, { method: 'DELETE' }).then(noContent)
}

// --- Knowledge bases (independent of any Environment) ---

export function listKnowledgeBases(): Promise<KnowledgeBase[]> {
  return fetch('/api/knowledge').then(json<KnowledgeBase[]>)
}

export function createKnowledgeBase(name: string, description?: string): Promise<KnowledgeBase> {
  return fetch('/api/knowledge', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, description }),
  }).then(json<KnowledgeBase>)
}

export function getKnowledgeBase(name: string): Promise<KnowledgeBase> {
  return fetch(`/api/knowledge/${encodeURIComponent(name)}`).then(json<KnowledgeBase>)
}

export function deleteKnowledgeBase(name: string): Promise<void> {
  return fetch(`/api/knowledge/${encodeURIComponent(name)}`, { method: 'DELETE' }).then(noContent)
}

export function addKnowledgeRecords(
  name: string,
  records: { title: string; content: string; tags?: string[] }[],
): Promise<KnowledgeBase> {
  return fetch(`/api/knowledge/${encodeURIComponent(name)}/records`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ records }),
  }).then(json<KnowledgeBase>)
}

export function updateKnowledgeRecord(
  name: string,
  index: number,
  record: { title: string; content: string; tags?: string[] },
): Promise<KnowledgeBase> {
  return fetch(`/api/knowledge/${encodeURIComponent(name)}/records/${index}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(record),
  }).then(json<KnowledgeBase>)
}

export function deleteKnowledgeRecord(name: string, index: number): Promise<void> {
  return fetch(`/api/knowledge/${encodeURIComponent(name)}/records/${index}`, { method: 'DELETE' }).then(noContent)
}

export function launchEnvironment(name: string, instanceName?: string): Promise<Instance> {
  return fetch(`/api/environments/${encodeURIComponent(name)}/launch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ instanceName }),
  }).then(json<Instance>)
}

export function listInstances(): Promise<Instance[]> {
  return fetch('/api/environments/instances').then(json<Instance[]>)
}

export async function stopInstance(id: string): Promise<void> {
  const res = await fetch(`/api/environments/instances/${encodeURIComponent(id)}/stop`, { method: 'POST' })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error ?? `request failed with status ${res.status}`)
  }
}

export function startExec(instanceId: string, command: string): Promise<Exec> {
  return fetch(`/api/environments/instances/${encodeURIComponent(instanceId)}/exec`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ command }),
  }).then(json<Exec>)
}

export function getExec(instanceId: string, execId: string): Promise<Exec> {
  return fetch(`/api/environments/instances/${encodeURIComponent(instanceId)}/execs/${encodeURIComponent(execId)}`).then(
    json<Exec>,
  )
}

export function listAgents(): Promise<Agent[]> {
  return fetch('/api/agents').then(json<Agent[]>)
}

export function saveAgent(name: string, graph: AgentGraph, environment?: string, description?: string): Promise<Agent> {
  return fetch('/api/agents', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, graph, environment, description }),
  }).then(json<Agent>)
}

export function getAgent(name: string): Promise<Agent> {
  return fetch(`/api/agents/${encodeURIComponent(name)}`).then(json<Agent>)
}

export function deleteAgent(name: string): Promise<void> {
  return fetch(`/api/agents/${encodeURIComponent(name)}`, { method: 'DELETE' }).then(noContent)
}

export function startAgentRun(name: string): Promise<AgentRun> {
  return fetch(`/api/agents/${encodeURIComponent(name)}/runs`, { method: 'POST' }).then(json<AgentRun>)
}

export function sendAgentMessage(runId: string, message: string): Promise<ChatMessage> {
  return fetch(`/api/agents/runs/${encodeURIComponent(runId)}/messages`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
  }).then(json<ChatMessage>)
}

export function stopAgentRun(runId: string): Promise<void> {
  return fetch(`/api/agents/runs/${encodeURIComponent(runId)}/stop`, { method: 'POST' }).then(() => undefined)
}

export function getAgentRun(id: string): Promise<AgentRun> {
  return fetch(`/api/agents/runs/${encodeURIComponent(id)}`).then(json<AgentRun>)
}

// startAgentDebugRun begins a paused debug session. graph/environment come
// straight from the canvas's current (possibly unsaved) state rather than
// the agent's saved definition, so debugging never requires saving first.
export function startAgentDebugRun(name: string, graph: AgentGraph, environment?: string): Promise<DebugState> {
  return fetch(`/api/agents/${encodeURIComponent(name)}/debug`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ graph, environment }),
  }).then(json<DebugState>)
}

// sendAgentDebugMessage starts a new turn: the input node becomes pending,
// ready for the first stepAgentDebugRun call.
export function sendAgentDebugMessage(id: string, message: string): Promise<DebugState> {
  return fetch(`/api/agents/debug/${encodeURIComponent(id)}/messages`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
  }).then(json<DebugState>)
}

// stepAgentDebugRun executes the session's pending node.
export function stepAgentDebugRun(id: string): Promise<DebugState> {
  return fetch(`/api/agents/debug/${encodeURIComponent(id)}/step`, { method: 'POST' }).then(json<DebugState>)
}

// retryAgentDebugRun re-executes the session's most recently stepped node
// with the exact input it saw, discarding what that attempt produced.
export function retryAgentDebugRun(id: string): Promise<DebugState> {
  return fetch(`/api/agents/debug/${encodeURIComponent(id)}/retry`, { method: 'POST' }).then(json<DebugState>)
}

export function getAgentDebugRun(id: string): Promise<DebugState> {
  return fetch(`/api/agents/debug/${encodeURIComponent(id)}`).then(json<DebugState>)
}

export function stopAgentDebugRun(id: string): Promise<void> {
  return fetch(`/api/agents/debug/${encodeURIComponent(id)}/stop`, { method: 'POST' }).then(() => undefined)
}

export function listEvaluations(): Promise<Evaluation[]> {
  return fetch('/api/evaluations').then(json<Evaluation[]>)
}

// saveEvaluation creates a new evaluation with no test cases at all —
// they're added afterward from its detail page, the same way a Benchmark
// starts empty.
export function saveEvaluation(evaluation: { name: string; environment?: string }): Promise<Evaluation> {
  return fetch('/api/evaluations', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(evaluation),
  }).then(json<Evaluation>)
}

export function getEvaluation(name: string): Promise<Evaluation> {
  return fetch(`/api/evaluations/${encodeURIComponent(name)}`).then(json<Evaluation>)
}

export function deleteEvaluation(name: string): Promise<void> {
  return fetch(`/api/evaluations/${encodeURIComponent(name)}`, { method: 'DELETE' }).then(noContent)
}

// updateEvaluationConfig updates only the Environment binding — a live
// setting, not versioned content, so this never touches test cases.
export function updateEvaluationConfig(name: string, environment: string): Promise<Evaluation> {
  return fetch(`/api/evaluations/${encodeURIComponent(name)}/config`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ environment }),
  }).then(json<Evaluation>)
}

export function startEvaluationRun(name: string, version: number, agentNames: string[]): Promise<EvaluationRun> {
  return fetch(`/api/evaluations/${encodeURIComponent(name)}/runs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ version, agentNames }),
  }).then(json<EvaluationRun>)
}

export function publishEvaluationVersion(name: string): Promise<EvaluationVersion> {
  return fetch(`/api/evaluations/${encodeURIComponent(name)}/versions`, { method: 'POST' }).then(json<EvaluationVersion>)
}

export function listEvaluationVersions(name: string): Promise<EvaluationVersion[]> {
  return fetch(`/api/evaluation-versions/${encodeURIComponent(name)}`).then(json<EvaluationVersion[]>)
}

export function listEvaluationRuns(): Promise<EvaluationRun[]> {
  return fetch('/api/evaluations/runs').then(json<EvaluationRun[]>)
}

export function getEvaluationRun(id: string): Promise<EvaluationRun> {
  return fetch(`/api/evaluations/runs/${encodeURIComponent(id)}`).then(json<EvaluationRun>)
}

export function getEvaluationResults(name: string): Promise<EvaluationRunResult[]> {
  return fetch(`/api/evaluation-results/${encodeURIComponent(name)}`).then(json<EvaluationRunResult[]>)
}

export function addEvaluationTestCases(name: string, testCases: TestCase[]): Promise<TestCase[]> {
  return fetch(`/api/evaluations/${encodeURIComponent(name)}/test-cases`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ testCases }),
  }).then(json<TestCase[]>)
}

export function updateEvaluationTestCase(name: string, index: number, testCase: TestCase): Promise<TestCase> {
  return fetch(`/api/evaluations/${encodeURIComponent(name)}/test-cases/${index}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(testCase),
  }).then(json<TestCase>)
}

export function deleteEvaluationTestCase(name: string, index: number): Promise<void> {
  return fetch(`/api/evaluations/${encodeURIComponent(name)}/test-cases/${index}`, { method: 'DELETE' }).then(noContent)
}

export function generateEvaluationTestCases(
  name: string,
  req: { model: string; seedPrompt: string; assertions: Assertion[]; tags?: string[]; count: number },
): Promise<TestCase[]> {
  return fetch(`/api/evaluations/${encodeURIComponent(name)}/test-cases/generate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  }).then(json<TestCase[]>)
}

export function listBenchmarks(): Promise<Benchmark[]> {
  return fetch('/api/benchmarks').then(json<Benchmark[]>)
}

export function saveBenchmark(benchmark: { name: string; testCases: TestCase[] }): Promise<Benchmark> {
  return fetch('/api/benchmarks', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(benchmark),
  }).then(json<Benchmark>)
}

export function getBenchmark(name: string): Promise<Benchmark> {
  return fetch(`/api/benchmarks/${encodeURIComponent(name)}`).then(json<Benchmark>)
}

export function deleteBenchmark(name: string): Promise<void> {
  return fetch(`/api/benchmarks/${encodeURIComponent(name)}`, { method: 'DELETE' }).then(noContent)
}

export function startBenchmarkRun(name: string, version: number, modelNames: string[]): Promise<BenchmarkRun> {
  return fetch(`/api/benchmarks/${encodeURIComponent(name)}/runs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ version, modelNames }),
  }).then(json<BenchmarkRun>)
}

export function publishBenchmarkVersion(name: string): Promise<BenchmarkVersion> {
  return fetch(`/api/benchmarks/${encodeURIComponent(name)}/versions`, { method: 'POST' }).then(json<BenchmarkVersion>)
}

export function listBenchmarkVersions(name: string): Promise<BenchmarkVersion[]> {
  return fetch(`/api/benchmark-versions/${encodeURIComponent(name)}`).then(json<BenchmarkVersion[]>)
}

export function listBenchmarkRuns(): Promise<BenchmarkRun[]> {
  return fetch('/api/benchmarks/runs').then(json<BenchmarkRun[]>)
}

export function getBenchmarkRun(id: string): Promise<BenchmarkRun> {
  return fetch(`/api/benchmarks/runs/${encodeURIComponent(id)}`).then(json<BenchmarkRun>)
}

export function getBenchmarkResults(name: string): Promise<BenchmarkRunResult[]> {
  return fetch(`/api/benchmark-results/${encodeURIComponent(name)}`).then(json<BenchmarkRunResult[]>)
}

export function addTestCases(name: string, testCases: TestCase[]): Promise<TestCase[]> {
  return fetch(`/api/benchmarks/${encodeURIComponent(name)}/test-cases`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ testCases }),
  }).then(json<TestCase[]>)
}

export function updateTestCase(name: string, index: number, testCase: TestCase): Promise<TestCase> {
  return fetch(`/api/benchmarks/${encodeURIComponent(name)}/test-cases/${index}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(testCase),
  }).then(json<TestCase>)
}

export function deleteTestCase(name: string, index: number): Promise<void> {
  return fetch(`/api/benchmarks/${encodeURIComponent(name)}/test-cases/${index}`, { method: 'DELETE' }).then(noContent)
}

export function generateTestCases(
  name: string,
  req: { model: string; seedPrompt: string; assertions: Assertion[]; tags?: string[]; count: number },
): Promise<TestCase[]> {
  return fetch(`/api/benchmarks/${encodeURIComponent(name)}/test-cases/generate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  }).then(json<TestCase[]>)
}

export interface SystemInfo {
  version: string
  registryRoot: string
}

export function getSystemInfo(): Promise<SystemInfo> {
  return fetch('/api/system').then(json<SystemInfo>)
}
