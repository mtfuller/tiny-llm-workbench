import type { AgentNodeData, KnowledgeBase, Tool, Workspace } from './api'

// GraphIssue is one problem found in the agent graph. `error` blocks Run and
// Debug; `warning` is advisory (surfaced, but the graph can still run).
export interface GraphIssue {
  severity: 'error' | 'warning'
  message: string
  nodeId?: string
}

// Minimal node/edge shapes (mirrors TemplateField.tsx) — AgentEditor works
// with React Flow's Node/Edge, whose `type` is optional.
interface GraphNodeLike {
  id: string
  type?: string
  data: AgentNodeData
}
interface GraphEdgeLike {
  source: string
  sourceHandle?: string | null
  target: string
}

interface ValidateArgs {
  nodes: GraphNodeLike[]
  edges: GraphEdgeLike[]
  // The agent's access: a bound TEST workspace, and the pools a tool /
  // knowledge / agent node picks from (names into the global catalogs).
  workspace: string
  agentTools: string[]
  agentKnowledgeBases: string[]
  toolCatalog: Tool[]
  knowledgeBases: KnowledgeBase[]
  workspaces: Workspace[]
}

const TEMPLATE_REF = /\{\{\s*([^{}]+?)\s*\}\}/g

// label returns a node's display name for messages, falling back to its type.
function label(n: GraphNodeLike): string {
  const name = n.data.name?.trim()
  return name ? `"${name}"` : `(unnamed ${n.type ?? 'node'})`
}

// templateStringsFor returns every user-authored template field on a node
// whose {{...}} references should resolve to a real node name.
function templateStringsFor(n: GraphNodeLike): string[] {
  const d = n.data
  switch (n.type) {
    case 'prompt':
      return [d.systemPrompt, d.promptTemplate].filter((s): s is string => !!s)
    case 'condition':
    case 'switch':
      return [d.matchTemplate].filter((s): s is string => !!s)
    case 'state':
      return [d.stateValue].filter((s): s is string => !!s)
    case 'say':
      return [d.sayTemplate].filter((s): s is string => !!s)
    case 'knowledge':
      return [d.knowledgeQuery].filter((s): s is string => !!s)
    case 'agent':
      return [d.agentInstructions].filter((s): s is string => !!s)
    case 'tool':
      return Object.values(d.toolArgs ?? {}).filter((s): s is string => !!s)
    default:
      return []
  }
}

// validateGraph runs on every edit. It mirrors what internal/agents would
// reject at run time (missing model, unpaired loop end, bad template
// reference, …) plus softer "this probably isn't what you meant" warnings,
// so problems surface before a run instead of as a mid-turn error.
export function validateGraph({
  nodes,
  edges,
  workspace,
  agentTools,
  agentKnowledgeBases,
  toolCatalog,
  knowledgeBases,
  workspaces,
}: ValidateArgs): GraphIssue[] {
  const issues: GraphIssue[] = []
  const add = (severity: GraphIssue['severity'], message: string, nodeId?: string) =>
    issues.push({ severity, message, nodeId })

  const toolSet = new Set(agentTools)
  const kbSet = new Set(agentKnowledgeBases)

  const byId = new Map(nodes.map((n) => [n.id, n]))
  const names = nodes.map((n) => n.data.name?.trim()).filter((s): s is string => !!s)
  const nameSet = new Set(names)
  const loopStartNames = new Set(nodes.filter((n) => n.type === 'loop_start').map((n) => n.data.name?.trim()).filter(Boolean))
  const loopStartCount = nodes.filter((n) => n.type === 'loop_start').length
  const outgoing = (id: string, handle?: string) =>
    edges.some((e) => e.source === id && (handle === undefined || (e.sourceHandle ?? '') === handle))
  const hasInbound = (id: string) => edges.some((e) => e.target === id)

  // --- structural ---------------------------------------------------------
  const inputs = nodes.filter((n) => n.type === 'input')
  if (inputs.length === 0) add('error', 'The graph needs an Input node.')
  if (inputs.length > 1) inputs.slice(1).forEach((n) => add('error', 'Only one Input node is allowed.', n.id))
  if (inputs.length === 1 && nodes.length > 1 && !outgoing(inputs[0].id))
    add('warning', 'The Input node is not connected to anything.', inputs[0].id)

  const seen = new Set<string>()
  for (const name of names) {
    if (seen.has(name)) {
      const dupe = nodes.find((n) => n.data.name?.trim() === name)
      add('error', `More than one node is named "${name}" — names must be unique.`, dupe?.id)
    }
    seen.add(name)
  }

  for (const e of edges) {
    if (!byId.has(e.source) || !byId.has(e.target)) add('error', 'An edge points at a node that no longer exists.')
  }

  // --- per-node config --------------------------------------------------
  for (const n of nodes) {
    const d = n.data
    switch (n.type) {
      case 'prompt':
        if (!d.model?.trim()) add('error', `Prompt node ${label(n)} has no model set.`, n.id)
        if (d.outputSchema?.trim()) {
          try {
            JSON.parse(d.outputSchema)
          } catch {
            add('error', `Prompt node ${label(n)}: the output schema is not valid JSON.`, n.id)
          }
        }
        break

      case 'condition': {
        if (!d.conditionType) {
          add('error', `Condition node ${label(n)} has no check type selected.`, n.id)
          break
        }
        const v = d.conditionValue?.trim()
        if (d.conditionType === 'json_schema') {
          if (!v) add('error', `Condition node ${label(n)} needs a JSON schema.`, n.id)
          else {
            try {
              JSON.parse(v)
            } catch {
              add('error', `Condition node ${label(n)}: the JSON schema is not valid JSON.`, n.id)
            }
          }
        } else if (!v) {
          add('error', `Condition node ${label(n)} needs a value to check for.`, n.id)
        }
        if (d.conditionType === 'similarity') {
          const t = d.conditionThreshold
          if (t === undefined || t <= 0 || t > 1)
            add('error', `Condition node ${label(n)} needs a similarity threshold between 0 and 1.`, n.id)
        }
        if (!outgoing(n.id, 'pass') && !outgoing(n.id, 'fail'))
          add('warning', `Condition node ${label(n)} has neither branch connected.`, n.id)
        break
      }

      case 'switch': {
        const cases = (d.switchCases ?? []).map((c) => c.value.trim())
        if (cases.length === 0 || cases.every((v) => !v)) {
          add('error', `Switch node ${label(n)} has no cases.`, n.id)
          break
        }
        if (cases.some((v) => !v)) add('error', `Switch node ${label(n)} has an empty case value.`, n.id)
        const dupes = cases.filter((v, i) => v && cases.indexOf(v) !== i)
        if (dupes.length) add('error', `Switch node ${label(n)} has duplicate case "${dupes[0]}".`, n.id)
        for (const v of new Set(cases)) {
          if (v && !outgoing(n.id, v)) add('warning', `Switch node ${label(n)}: case "${v}" has no outgoing edge.`, n.id)
        }
        if (!outgoing(n.id, 'default'))
          add('warning', `Switch node ${label(n)} has nothing wired to its "default" handle.`, n.id)
        break
      }

      case 'loop_start':
        if ((d.loopMaxIterations ?? 0) < 1)
          add('warning', `Loop start ${label(n)} has no max iterations set — it will default to 10.`, n.id)
        if (!outgoing(n.id, 'body'))
          add('warning', `Loop start ${label(n)} has nothing connected to its "body" handle.`, n.id)
        if (!outgoing(n.id, 'done'))
          add(
            'warning',
            `Loop start ${label(n)} has nothing wired to its "done" handle — when it hits the max, the turn just ends.`,
            n.id,
          )
        break

      case 'loop_end': {
        const target = d.loopStartName?.trim()
        const resolved = target ? loopStartNames.has(target) : loopStartCount === 1
        if (!resolved)
          add(
            'error',
            target
              ? `Loop end ${label(n)} points to "${target}", which is not a Loop start.`
              : `Loop end ${label(n)} does not point to a Loop start.`,
            n.id,
          )
        break
      }

      case 'tool': {
        if (!workspace) {
          add('error', `Tool node ${label(n)} needs the agent to have a workspace (Agent settings).`, n.id)
          break
        }
        if (!d.toolName) {
          add('error', `Tool node ${label(n)} has no tool selected.`, n.id)
          break
        }
        if (!toolSet.has(d.toolName)) {
          add('error', `Tool node ${label(n)}: tool "${d.toolName}" is not in the agent's tools (Agent settings).`, n.id)
          break
        }
        const tool = toolCatalog.find((t) => t.name === d.toolName)
        if (!tool) {
          add('error', `Tool node ${label(n)}: tool "${d.toolName}" no longer exists in the catalog.`, n.id)
          break
        }
        for (const p of tool.parameters) {
          if (p.required && !d.toolArgs?.[p.name]?.trim())
            add('error', `Tool node ${label(n)} is missing required parameter "${p.name}".`, n.id)
        }
        break
      }

      case 'agent': {
        if (!d.agentModel?.trim()) add('error', `Agent node ${label(n)} has no model set.`, n.id)
        if (d.agentOutputSchema?.trim()) {
          try {
            JSON.parse(d.agentOutputSchema)
          } catch {
            add('error', `Agent node ${label(n)}: the output schema is not valid JSON.`, n.id)
          }
        }
        const picked = d.agentTools ?? []
        const pickedKbs = d.agentKnowledgeBases ?? []
        if (picked.length > 0 && !workspace)
          add('error', `Agent node ${label(n)} has tools selected but the agent has no workspace.`, n.id)
        for (const t of picked) {
          if (!toolSet.has(t))
            add('error', `Agent node ${label(n)}: tool "${t}" is not in the agent's tools (Agent settings).`, n.id)
        }
        for (const kb of pickedKbs) {
          if (!kbSet.has(kb))
            add('error', `Agent node ${label(n)}: knowledge base "${kb}" is not in the agent's set (Agent settings).`, n.id)
        }
        if (picked.length === 0 && pickedKbs.length === 0)
          add(
            'warning',
            `Agent node ${label(n)} has no tools or knowledge bases selected — it will only reply, never act.`,
            n.id,
          )

        const tmpl = d.agentPromptTemplate?.trim()
        if (tmpl) {
          if (!tmpl.includes('{{transcript}}'))
            add(
              'warning',
              `Agent node ${label(n)}: the prompt template has no {{transcript}} — the model won't see its own past tool calls and may loop until it hits the max iterations.`,
              n.id,
            )
          if ((picked.length > 0 || pickedKbs.length > 0) && !/\bACTION\b/.test(tmpl) && !/\bFINAL\b/.test(tmpl))
            add(
              'warning',
              `Agent node ${label(n)}: the prompt template has no ACTION/FINAL protocol text — the model won't know how to call the tools or answer.`,
              n.id,
            )
          const scalars = new Set([
            'instructions',
            'tools',
            'knowledge',
            'history',
            'transcript',
            'input',
            'tool_names',
            'args_example',
          ])
          const conditionals = new Set(['instructions', 'tools', 'knowledge', 'history', 'transcript', 'actions'])
          for (const m of tmpl.matchAll(/\{\{([#/]?)\s*(\w+)\s*\}\}/g)) {
            const [, marker, name] = m
            const ok = marker ? conditionals.has(name) : scalars.has(name)
            if (!ok)
              add('warning', `Agent node ${label(n)}: unknown template placeholder {{${marker}${name}}} — it will render literally.`, n.id)
          }
        }
        break
      }

      case 'knowledge':
        if (!d.knowledgeBaseName) add('error', `Knowledge node ${label(n)} has no knowledge base selected.`, n.id)
        else if (!kbSet.has(d.knowledgeBaseName))
          add(
            'error',
            `Knowledge node ${label(n)}: knowledge base "${d.knowledgeBaseName}" is not in the agent's set (Agent settings).`,
            n.id,
          )
        break

      case 'say':
        if (d.sayFinal && !d.sayTemplate?.trim())
          add('warning', `Say node ${label(n)} is marked final but has no text — it'll echo the previous node's output.`, n.id)
        break

      case 'input':
        break

      default:
        add('warning', `Node ${label(n)} has an unrecognised type "${n.type}".`, n.id)
    }

    // Orphan check — a non-input node with nothing wired into it never runs.
    if (n.type !== 'input' && !hasInbound(n.id))
      add('warning', `Node ${label(n)} has nothing connected into it — it will never run.`, n.id)

    // Template references must name a real node.
    for (const tpl of templateStringsFor(n)) {
      for (const m of tpl.matchAll(TEMPLATE_REF)) {
        const ref = m[1].split('.')[0].trim()
        if (ref && !nameSet.has(ref))
          add('error', `Node ${label(n)} references {{${ref}}}, but no node is named "${ref}".`, n.id)
      }
    }
  }

  // Agent-level tool/KB references that no longer resolve against the catalogs.
  for (const t of agentTools) {
    if (!toolCatalog.some((c) => c.name === t))
      add('warning', `The agent's tool set includes "${t}", which no longer exists in the catalog.`)
  }
  for (const kb of agentKnowledgeBases) {
    if (!knowledgeBases.some((c) => c.name === kb))
      add('warning', `The agent's knowledge set includes "${kb}", which no longer exists.`)
  }

  // Workspace binding that no longer resolves (or isn't a test workspace).
  if (workspace && !workspaces.some((w) => w.name === workspace && w.type === 'test'))
    add('error', `This agent is bound to workspace "${workspace}", which no longer exists or isn't a test workspace.`)

  // More than one Say node marked final — whichever runs last on the taken
  // path wins, which is easy to get wrong.
  const finalSays = nodes.filter((n) => n.type === 'say' && n.data.sayFinal)
  if (finalSays.length > 1)
    add('warning', `${finalSays.length} Say nodes are marked as the final answer — whichever runs last wins.`)

  // De-dupe identical messages (e.g. the same bad {{ref}} in two fields).
  const key = (i: GraphIssue) => `${i.severity}|${i.nodeId ?? ''}|${i.message}`
  const uniq = new Map(issues.map((i) => [key(i), i]))
  return [...uniq.values()].sort((a, b) => (a.severity === b.severity ? 0 : a.severity === 'error' ? -1 : 1))
}
