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

export interface Tool {
  name: string
  description?: string
  command: string
  parameters: ToolParameter[]
}

export interface Environment {
  name: string
  image: string
  tools: Tool[]
  mounts: Mount[]
  prebuilt: boolean
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

export type NodeType = 'input' | 'prompt' | 'decision' | 'tool' | 'output'

export interface AgentNodeData extends Record<string, unknown> {
  // name is a stable, user-editable, unique-within-the-graph display name —
  // shown on the canvas and used as the token a downstream node's template
  // references: {{name}} for this node's raw text output, or
  // {{name.property}} for a property of it if outputSchema made that output
  // parseable JSON. A node with no name isn't referenceable at all.
  name?: string

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

  keyword?: string
  // matchTemplate is templated text to search keyword within; falls back to
  // the previous node's raw output when empty.
  matchTemplate?: string

  // Tool nodes: toolName names a Tool declared on the agent's bound
  // Environment (see registry.Tool); each toolArgs value may itself contain
  // {{nodeName}}/{{nodeName.field}} template references.
  toolName?: string
  toolArgs?: Record<string, string>
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
}

export type AssertionType = 'contains' | 'not_contains' | 'regex' | 'json_schema' | 'similarity'

export interface Assertion {
  type: AssertionType
  value: string
  // threshold is only meaningful for "similarity" (the minimum required
  // similarity ratio, in (0, 1]).
  threshold?: number
}

export interface TestCase {
  id: string
  prompt: string
  assertions: Assertion[]
  // tags are only used by Benchmarks today, for filtering the test case
  // list — same role as Example.tags for datasets.
  tags?: string[]
}

export interface Evaluation {
  name: string
  environment?: string
  testCases: TestCase[]
  createdAt: string
}

export type EvalRunStatus = 'running' | 'succeeded' | 'failed'

export interface AssertionResult extends Assertion {
  passed: boolean
  error?: string
}

export interface TestCaseResult {
  testCaseId: string
  prompt: string
  reply: string
  assertions: AssertionResult[]
  passed: boolean
  error?: string
}

export interface AgentResult {
  agentName: string
  results: TestCaseResult[]
  passed: number
  total: number
}

export interface EvaluationRun {
  id: string
  evaluationName: string
  agentNames: string[]
  environmentName?: string
  instanceId?: string
  status: EvalRunStatus
  agentResults: AgentResult[]
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

export function addTool(name: string, tool: Tool): Promise<Tool> {
  return fetch(`/api/environments/${encodeURIComponent(name)}/tools`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(tool),
  }).then(json<Tool>)
}

export function updateTool(name: string, index: number, tool: Tool): Promise<Tool> {
  return fetch(`/api/environments/${encodeURIComponent(name)}/tools/${index}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(tool),
  }).then(json<Tool>)
}

export function deleteTool(name: string, index: number): Promise<void> {
  return fetch(`/api/environments/${encodeURIComponent(name)}/tools/${index}`, { method: 'DELETE' }).then(noContent)
}

export function tryTool(name: string, index: number, instanceId: string, args: Record<string, string>): Promise<Exec> {
  return fetch(`/api/environments/${encodeURIComponent(name)}/tools/${index}/try`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ instanceId, args }),
  }).then(json<Exec>)
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

export function listEvaluations(): Promise<Evaluation[]> {
  return fetch('/api/evaluations').then(json<Evaluation[]>)
}

export function saveEvaluation(evaluation: { name: string; environment?: string; testCases: TestCase[] }): Promise<Evaluation> {
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

export function startEvaluationRun(name: string, agentNames: string[]): Promise<EvaluationRun> {
  return fetch(`/api/evaluations/${encodeURIComponent(name)}/runs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ agentNames }),
  }).then(json<EvaluationRun>)
}

export function listEvaluationRuns(): Promise<EvaluationRun[]> {
  return fetch('/api/evaluations/runs').then(json<EvaluationRun[]>)
}

export function getEvaluationRun(id: string): Promise<EvaluationRun> {
  return fetch(`/api/evaluations/runs/${encodeURIComponent(id)}`).then(json<EvaluationRun>)
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
