import { Braces } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { AgentNodeData } from './api'

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
    if (node.type === 'prompt' && node.data.outputSchema) {
      for (const key of extractTopLevelProperties(node.data.outputSchema)) {
        options.push({ insert: `${name}.${key}`, label: `${name} → ${key}` })
      }
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
// the chosen one at the caller's field's cursor position on click. Mirrors
// TagFilterDropdown's portal-to-body popover pattern.
export function VariableMenuButton({ options, onInsert }: VariableMenuButtonProps) {
  const [open, setOpen] = useState(false)
  const [menuPos, setMenuPos] = useState<{ top: number; right: number } | null>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const rect = buttonRef.current?.getBoundingClientRect()
    // Anchor from the button's right edge, not its left: this button
    // typically sits at the right edge of the (narrow, fixed-width)
    // inspector sidebar, where a left-anchored popover would overflow the
    // viewport — right-anchoring against a coordinate that's always
    // on-screen guarantees the menu never gets clipped.
    if (rect) setMenuPos({ top: rect.bottom + 6, right: window.innerWidth - rect.right })

    const handleClick = (e: MouseEvent) => {
      const target = e.target as Node
      if (buttonRef.current?.contains(target) || menuRef.current?.contains(target)) return
      setOpen(false)
    }
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    const handleScroll = () => setOpen(false)
    document.addEventListener('mousedown', handleClick)
    document.addEventListener('keydown', handleKey)
    window.addEventListener('scroll', handleScroll, true)
    return () => {
      document.removeEventListener('mousedown', handleClick)
      document.removeEventListener('keydown', handleKey)
      window.removeEventListener('scroll', handleScroll, true)
    }
  }, [open])

  return (
    <div className="variable-menu-anchor">
      <button
        ref={buttonRef}
        type="button"
        className="icon-button"
        title="Insert a reference to an earlier node's output"
        aria-label="Insert variable"
        onClick={() => setOpen((o) => !o)}
      >
        <Braces size={14} />
      </button>
      {open &&
        menuPos &&
        createPortal(
          <div className="variable-menu" ref={menuRef} style={{ top: menuPos.top, right: menuPos.right }}>
            {options.length === 0 && <p className="variable-menu-empty">No named upstream nodes yet.</p>}
            {options.map((opt) => (
              <button
                key={opt.insert}
                type="button"
                className="variable-menu-item"
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
