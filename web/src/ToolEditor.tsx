import type { Tool, ToolParameter, ToolParameterType } from './api'

export interface DraftToolParameter {
  name: string
  type: ToolParameterType
  description: string
  required: boolean
}

export interface DraftTool {
  name: string
  description: string
  command: string
  parameters: DraftToolParameter[]
}

export function emptyToolParameter(): DraftToolParameter {
  return { name: '', type: 'string', description: '', required: true }
}

export function emptyTool(): DraftTool {
  return { name: '', description: '', command: '', parameters: [] }
}

// toDraftTool seeds the tool editor from a saved Tool.
export function toDraftTool(tool: Tool): DraftTool {
  return {
    name: tool.name,
    description: tool.description ?? '',
    command: tool.command,
    parameters: tool.parameters.map((p) => ({
      name: p.name,
      type: p.type,
      description: p.description ?? '',
      required: p.required,
    })),
  }
}

// toPayloadTool drops blank parameters, ready to send to addTool/updateTool.
export function toPayloadTool(draft: DraftTool): Tool {
  return {
    name: draft.name.trim(),
    description: draft.description.trim() || undefined,
    command: draft.command.trim(),
    parameters: draft.parameters
      .filter((p) => p.name.trim())
      .map(
        (p): ToolParameter => ({
          name: p.name.trim(),
          type: p.type,
          description: p.description.trim() || undefined,
          required: p.required,
        }),
      ),
  }
}

interface ToolParameterFieldsProps {
  parameters: DraftToolParameter[]
  onChange: (parameters: DraftToolParameter[]) => void
}

// ToolParameterFields renders the repeated parameter rows (name, type,
// description, required) for a single tool's I/O schema — a simple typed
// parameter list rather than full JSON Schema, matching the deterministic,
// non-LLM-graded style of the rest of the assertion/decision-node surface.
export function ToolParameterFields({ parameters, onChange }: ToolParameterFieldsProps) {
  const updateParameter = (i: number, patch: Partial<DraftToolParameter>) => {
    onChange(parameters.map((p, idx) => (idx === i ? { ...p, ...patch } : p)))
  }

  const addParameter = () => onChange([...parameters, emptyToolParameter()])
  const removeParameter = (i: number) => onChange(parameters.filter((_, idx) => idx !== i))

  return (
    <>
      {parameters.map((p, i) => (
        <div className="tool-param-row" key={i}>
          <input type="text" placeholder="name" value={p.name} onChange={(e) => updateParameter(i, { name: e.target.value })} />
          <select value={p.type} onChange={(e) => updateParameter(i, { type: e.target.value as ToolParameterType })}>
            <option value="string">string</option>
            <option value="number">number</option>
            <option value="boolean">boolean</option>
          </select>
          <input
            type="text"
            placeholder="description"
            value={p.description}
            onChange={(e) => updateParameter(i, { description: e.target.value })}
          />
          <label className="checkbox-label">
            <input type="checkbox" checked={p.required} onChange={(e) => updateParameter(i, { required: e.target.checked })} />
            required
          </label>
          <button type="button" className="danger-button" onClick={() => removeParameter(i)}>
            ×
          </button>
        </div>
      ))}
      <button type="button" className="button-secondary" onClick={addParameter}>
        + Parameter
      </button>
    </>
  )
}
