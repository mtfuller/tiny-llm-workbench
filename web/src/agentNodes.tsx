import { Handle, Position, type NodeProps, type Node } from '@xyflow/react'
import type { AgentNodeData } from './api'

type FlowNode = Node<AgentNodeData>

// The canvas flows top-to-bottom: every node takes its inbound edge on the
// top edge and emits on the bottom edge. Nodes with two outgoing handles
// (condition, loop start) place both along the bottom, offset horizontally.

// debugClass appends the pending/executed outline class a debug session
// sets on data.debugHighlight (see AgentEditor.tsx) — a no-op string when
// there's no active debug session.
function debugClass(data: AgentNodeData): string {
  if (data.debugHighlight === 'pending') return ' flow-node-pending'
  if (data.debugHighlight === 'executed') return ' flow-node-executed'
  return ''
}

function InputNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className={`flow-node flow-node-input${debugClass(data)}`}>
      <div className="flow-node-title">{data.name || 'Input'}</div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

// SchemaHandles renders the outgoing handles for a node that can constrain
// its output with a JSON schema: a single default handle normally, or a
// labelled out/fail pair once a schema is set (a mismatch routes to "fail").
function SchemaHandles({ hasSchema }: { hasSchema: boolean }) {
  if (!hasSchema) return <Handle type="source" position={Position.Bottom} />
  return (
    <>
      <Handle type="source" position={Position.Bottom} id="out" style={{ left: '32%' }} />
      <span className="flow-node-handle-label flow-node-handle-label-pass">out</span>
      <Handle type="source" position={Position.Bottom} id="fail" style={{ left: '68%' }} />
      <span className="flow-node-handle-label flow-node-handle-label-fail">fail</span>
    </>
  )
}

function PromptNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className={`flow-node flow-node-prompt${debugClass(data)}`}>
      <Handle type="target" position={Position.Top} />
      <div className="flow-node-title">{data.name || 'Prompt'}</div>
      <div className="flow-node-sub">{data.model || 'no model set'}</div>
      <SchemaHandles hasSchema={!!data.outputSchema?.trim()} />
    </div>
  )
}

// conditionSummary is a short read-only description of a condition node's
// check, mirroring formatAssertion in TestCaseEditor.tsx.
function conditionSummary(data: AgentNodeData): string {
  const v = data.conditionValue ?? ''
  switch (data.conditionType) {
    case 'contains':
      return `contains "${v}"`
    case 'not_contains':
      return `not contains "${v}"`
    case 'regex':
      return `matches /${v}/`
    case 'json_schema':
      return 'matches JSON schema'
    case 'similarity':
      return `similar to "${v}" (≥ ${Math.round((data.conditionThreshold ?? 0.85) * 100)}%)`
    default:
      return 'no check set'
  }
}

function ConditionNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className={`flow-node flow-node-condition${debugClass(data)}`}>
      <Handle type="target" position={Position.Top} />
      <div className="flow-node-title">{data.name || 'Condition'}</div>
      <div className="flow-node-sub">{conditionSummary(data)}</div>
      <Handle type="source" position={Position.Bottom} id="pass" style={{ left: '32%' }} />
      <span className="flow-node-handle-label flow-node-handle-label-pass">pass</span>
      <Handle type="source" position={Position.Bottom} id="fail" style={{ left: '68%' }} />
      <span className="flow-node-handle-label flow-node-handle-label-fail">fail</span>
    </div>
  )
}

// A switch node fans out to one handle per case value plus a "default"
// handle, spread evenly along the bottom edge.
function SwitchNode({ data }: NodeProps<FlowNode>) {
  const cases = (data.switchCases ?? []).filter((c) => c.value.trim())
  const handles = [...cases.map((c) => c.value.trim()), 'default']
  return (
    <div className={`flow-node flow-node-switch${debugClass(data)}`}>
      <Handle type="target" position={Position.Top} />
      <div className="flow-node-title">{data.name || 'Switch'}</div>
      <div className="flow-node-sub">
        {cases.length} case{cases.length === 1 ? '' : 's'} + default
      </div>
      {handles.map((h, i) => {
        const left = `${((i + 1) / (handles.length + 1)) * 100}%`
        return (
          <span key={h}>
            <Handle type="source" position={Position.Bottom} id={h} style={{ left }} />
            <span
              className={`flow-node-handle-label ${h === 'default' ? 'flow-node-handle-label-fail' : 'flow-node-handle-label-pass'}`}
              style={{ left }}
            >
              {h}
            </span>
          </span>
        )
      })}
    </div>
  )
}

function LoopStartNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className={`flow-node flow-node-loop${debugClass(data)}`}>
      <Handle type="target" position={Position.Top} />
      <div className="flow-node-title">{data.name || 'Loop start'}</div>
      <div className="flow-node-sub">max {data.loopMaxIterations || 10} iterations</div>
      <Handle type="source" position={Position.Bottom} id="body" style={{ left: '32%' }} />
      <span className="flow-node-handle-label flow-node-handle-label-pass">body</span>
      <Handle type="source" position={Position.Bottom} id="done" style={{ left: '68%' }} />
      <span className="flow-node-handle-label flow-node-handle-label-fail">done</span>
    </div>
  )
}

// A loop_end takes an inbound edge and, instead of a user-drawn outgoing
// edge, jumps back to its paired loop_start — AgentEditor renders that as a
// dashed edge. The source handle exists only so React Flow can anchor that
// dashed edge; it's on the side (so the back-edge sweeps up alongside the
// body) and hidden/non-connectable so users can't wire from it.
function LoopEndNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className={`flow-node flow-node-loop flow-node-loop-end${debugClass(data)}`}>
      <Handle type="target" position={Position.Top} />
      <div className="flow-node-title">{data.name || 'Loop end'}</div>
      <div className="flow-node-sub">↺ {data.loopStartName || 'pick a loop start'}</div>
      <Handle
        type="source"
        position={Position.Left}
        isConnectable={false}
        style={{ opacity: 0, pointerEvents: 'none' }}
      />
    </div>
  )
}

function StateNode({ data }: NodeProps<FlowNode>) {
  const op = data.stateOp || 'append'
  return (
    <div className={`flow-node flow-node-state${debugClass(data)}`}>
      <Handle type="target" position={Position.Top} />
      <div className="flow-node-title">{data.name || 'State'}</div>
      <div className="flow-node-sub">
        {op} {data.stateValue ? `"${data.stateValue}"` : 'previous output'}
      </div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

function ToolNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className={`flow-node flow-node-tool${debugClass(data)}`}>
      <Handle type="target" position={Position.Top} />
      <div className="flow-node-title">{data.name || 'Tool'}</div>
      <div className="flow-node-sub">{data.toolName || 'no tool selected'}</div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

function AgentNode({ data }: NodeProps<FlowNode>) {
  const toolCount = data.agentTools?.length ?? 0
  const kbCount = data.agentKnowledgeBases?.length ?? 0
  const capabilities = [`${toolCount} tool${toolCount === 1 ? '' : 's'}`]
  if (kbCount > 0) capabilities.push(`${kbCount} KB${kbCount === 1 ? '' : 's'}`)
  return (
    <div className={`flow-node flow-node-agent${debugClass(data)}`}>
      <Handle type="target" position={Position.Top} />
      <div className="flow-node-title">{data.name || 'Agent'}</div>
      <div className="flow-node-sub">
        {data.agentModel || 'no model set'} · {capabilities.join(', ')}
      </div>
      <SchemaHandles hasSchema={!!data.agentOutputSchema?.trim()} />
    </div>
  )
}

function KnowledgeNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className={`flow-node flow-node-knowledge${debugClass(data)}`}>
      <Handle type="target" position={Position.Top} />
      <div className="flow-node-title">{data.name || 'Knowledge'}</div>
      <div className="flow-node-sub">{data.knowledgeBaseName || 'no knowledge base selected'}</div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

export const nodeTypes = {
  input: InputNode,
  prompt: PromptNode,
  condition: ConditionNode,
  switch: SwitchNode,
  loop_start: LoopStartNode,
  loop_end: LoopEndNode,
  state: StateNode,
  tool: ToolNode,
  agent: AgentNode,
  knowledge: KnowledgeNode,
}
