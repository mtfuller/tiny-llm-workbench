export type ModelSource = 'ollama' | 'mlx' | 'binary' | string

export interface Model {
  name: string
  source: ModelSource
  size?: number
}

export interface Example {
  input: string
  output: string
}

export interface DatasetSummary {
  name: string
  createdAt: string
  pairCount: number
}

export interface DatasetDetail {
  name: string
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
}

export interface Environment {
  name: string
  image: string
  tools: string[]
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
  label?: string
  model?: string
  systemPrompt?: string
  keyword?: string
  command?: string
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

export type AssertionType = 'contains' | 'not_contains' | 'regex'

export interface Assertion {
  type: AssertionType
  value: string
}

export interface TestCase {
  id: string
  prompt: string
  assertions: Assertion[]
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

export function deleteModel(name: string, source: string): Promise<void> {
  return fetch(`/api/models/${encodeURIComponent(name)}?source=${encodeURIComponent(source)}`, {
    method: 'DELETE',
  }).then(noContent)
}

export function listDatasets(): Promise<DatasetSummary[]> {
  return fetch('/api/datasets').then(json<DatasetSummary[]>)
}

export function createDataset(name: string): Promise<DatasetSummary> {
  return fetch('/api/datasets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
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

export function createEnvironment(env: { name: string; image: string; tools: string[]; mounts: Mount[] }): Promise<Environment> {
  return fetch('/api/environments', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(env),
  }).then(json<Environment>)
}

export function deleteEnvironment(name: string): Promise<void> {
  return fetch(`/api/environments/${encodeURIComponent(name)}`, { method: 'DELETE' }).then(noContent)
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

export function saveAgent(name: string, graph: AgentGraph, environment?: string): Promise<Agent> {
  return fetch('/api/agents', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, graph, environment }),
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

export interface SystemInfo {
  version: string
  registryRoot: string
  ollamaBaseUrl: string
}

export function getSystemInfo(): Promise<SystemInfo> {
  return fetch('/api/system').then(json<SystemInfo>)
}
