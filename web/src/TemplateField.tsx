import { Braces } from 'lucide-react'
import { createPortal } from 'react-dom'
import type { AgentNodeData } from './api'
import { usePopoverMenu } from './usePopoverMenu'

export interface VariableOption {
  insert: string // e.g. "Classifier" or "Classifier.city" — goes inside {{...}}
  label: string // e.g. "Classifier" or "Classifier → city"
}

// Minimal shapes rather than api.ts's AgentNode/AgentEdge (whose `type` is a
// required NodeType): AgentEditor.tsx works with React Flow's own Node/Edge
// types, whose `type` is optional, so this only asks for the fields it
// actually needs and stays structurally compatible with both.
interface GraphNodeLike {
  id: string
  type?: string
  data: AgentNodeData
}
interface GraphEdgeLike {
  source: string
  target: string
}

// upstreamVariableOptions computes every {{...}} reference a node at
// nodeId can use: the raw output of every named node reachable by walking
// edges backward (its true ancestors, not just its immediate predecessor —
// see internal/agents.Engine's runContext), plus, for an ancestor prompt
// node with an outputSchema configured, one option per top-level property
// declared in that schema.
export function upstreamVariableOptions(nodes: GraphNodeLike[], edges: GraphEdgeLike[], nodeId: string): VariableOption[] {
  const byId = new Map(nodes.map((n) => [n.id, n]))
  const visited = new Set<string>([nodeId])
  const queue = [nodeId]
  const ancestors: GraphNodeLike[] = []

  while (queue.length > 0) {
    const current = queue.shift()!
    for (const edge of edges) {
      if (edge.target !== current || visited.has(edge.source)) continue
      visited.add(edge.source)
      const node = byId.get(edge.source)
      if (node) {
        ancestors.push(node)
        queue.push(edge.source)
      }
    }
  }

  const options: VariableOption[] = []
  for (const node of ancestors) {
    const name = node.data.name?.trim()
    if (!name) continue
    options.push({ insert: name, label: name })
    // A prompt or agent node with an output schema exposes each top-level
    // property as {{Name.property}} (the engine validates + parses its reply).
    const schema =
      node.type === 'prompt' ? node.data.outputSchema : node.type === 'agent' ? node.data.agentOutputSchema : undefined
    if (schema) {
      for (const key of extractTopLevelProperties(schema)) {
        options.push({ insert: `${name}.${key}`, label: `${name} → ${key}` })
      }
    }
    // A loop_start exposes its 1-based pass count as {{Name.iteration}} (the
    // engine special-cases it — see runcontext.go).
    if (node.type === 'loop_start') {
      options.push({ insert: `${name}.iteration`, label: `${name} → iteration` })
    }
  }
  return options
}

function extractTopLevelProperties(schemaText: string): string[] {
  try {
    const schema = JSON.parse(schemaText) as { properties?: Record<string, unknown> }
    return schema.properties ? Object.keys(schema.properties) : []
  } catch {
    return []
  }
}

// insertAtCursor splices snippet into value at el's current cursor
// position (or the end, if unknown), calls onChange with the result, and
// restores focus with the cursor placed right after the inserted text.
export function insertAtCursor(
  el: HTMLInputElement | HTMLTextAreaElement | null,
  value: string,
  snippet: string,
  onChange: (next: string) => void,
) {
  const start = el?.selectionStart ?? value.length
  const end = el?.selectionEnd ?? value.length
  const next = value.slice(0, start) + snippet + value.slice(end)
  onChange(next)

  if (!el) return
  requestAnimationFrame(() => {
    el.focus()
    const pos = start + snippet.length
    el.setSelectionRange(pos, pos)
  })
}

interface VariableMenuButtonProps {
  options: VariableOption[]
  onInsert: (snippet: string) => void
}

// VariableMenuButton is a small icon button that opens a popover listing
// every {{...}} reference available at this point in the graph, inserting
// the chosen one at the caller's field's cursor position on click.
export function VariableMenuButton({ options, onInsert }: VariableMenuButtonProps) {
  const { open, setOpen, position, triggerRef, menuRef } = usePopoverMenu()

  return (
    <div className="dropdown-menu-anchor">
      <button
        ref={triggerRef}
        type="button"
        className="icon-button"
        title="Insert a reference to an earlier node's output"
        aria-label="Insert variable"
        onClick={() => setOpen((o) => !o)}
      >
        <Braces size={14} />
      </button>
      {open &&
        position &&
        createPortal(
          <div className="dropdown-menu" ref={menuRef} style={{ top: position.top, right: position.right }}>
            {options.length === 0 && <p className="dropdown-menu-empty">No named upstream nodes yet.</p>}
            {options.map((opt) => (
              <button
                key={opt.insert}
                type="button"
                className="dropdown-menu-item dropdown-menu-item-mono"
                onClick={() => {
                  onInsert(`{{${opt.insert}}}`)
                  setOpen(false)
                }}
              >
                {opt.label}
              </button>
            ))}
          </div>,
          document.body,
        )}
    </div>
  )
}
