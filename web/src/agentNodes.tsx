import { Handle, Position, type NodeProps, type Node } from '@xyflow/react'
import type { AgentNodeData } from './api'

type FlowNode = Node<AgentNodeData>

function InputNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className="flow-node flow-node-input">
      <div className="flow-node-title">{data.name || 'Input'}</div>
      <Handle type="source" position={Position.Right} />
    </div>
  )
}

function PromptNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className="flow-node flow-node-prompt">
      <Handle type="target" position={Position.Left} />
      <div className="flow-node-title">{data.name || 'Prompt'}</div>
      <div className="flow-node-sub">{data.model || 'no model set'}</div>
      <Handle type="source" position={Position.Right} />
    </div>
  )
}

function DecisionNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className="flow-node flow-node-decision">
      <Handle type="target" position={Position.Left} />
      <div className="flow-node-title">{data.name || 'Decision'}</div>
      <div className="flow-node-sub">{data.keyword ? `contains "${data.keyword}"` : 'no keyword set'}</div>
      <Handle type="source" position={Position.Right} id="yes" style={{ top: '35%' }} />
      <span className="flow-node-handle-label flow-node-handle-label-yes">yes</span>
      <Handle type="source" position={Position.Right} id="no" style={{ top: '75%' }} />
      <span className="flow-node-handle-label flow-node-handle-label-no">no</span>
    </div>
  )
}

function ToolNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className="flow-node flow-node-tool">
      <Handle type="target" position={Position.Left} />
      <div className="flow-node-title">{data.name || 'Tool'}</div>
      <div className="flow-node-sub">{data.toolName || 'no tool selected'}</div>
      <Handle type="source" position={Position.Right} />
    </div>
  )
}

function KnowledgeNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className="flow-node flow-node-knowledge">
      <Handle type="target" position={Position.Left} />
      <div className="flow-node-title">{data.name || 'Knowledge'}</div>
      <div className="flow-node-sub">{data.knowledgeBaseName || 'no knowledge base selected'}</div>
      <Handle type="source" position={Position.Right} />
    </div>
  )
}

function OutputNode({ data }: NodeProps<FlowNode>) {
  return (
    <div className="flow-node flow-node-output">
      <Handle type="target" position={Position.Left} />
      <div className="flow-node-title">{data.name || 'Output'}</div>
    </div>
  )
}

export const nodeTypes = {
  input: InputNode,
  prompt: PromptNode,
  decision: DecisionNode,
  tool: ToolNode,
  knowledge: KnowledgeNode,
  output: OutputNode,
}
