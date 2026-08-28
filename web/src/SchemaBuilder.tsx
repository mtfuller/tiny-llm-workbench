import { useState } from 'react'
import { Braces, Plus, Trash2 } from 'lucide-react'
import LineNumberedTextarea from './LineNumberedTextarea'

// SchemaBuilder is a compact tree editor for a JSON Schema describing an
// object's properties — used for a Prompt node's output schema and a
// Condition node's json_schema check. It emits a JSON Schema *string*
// (the shape the engine's internal/assertions.ValidateJSONSchema expects),
// or "" when no property has been defined (i.e. no schema at all). A raw-JSON
// escape hatch handles anything the builder can't represent (enum, oneOf,
// $ref, …).

type PrimType = 'string' | 'number' | 'integer' | 'boolean'
type FieldType = PrimType | 'object' | 'array'
const PRIMS: PrimType[] = ['string', 'number', 'integer', 'boolean']

interface SchemaField {
  id: string
  name: string
  type: FieldType
  required: boolean
  children: SchemaField[] // when type === 'object'
  itemType: PrimType | 'object' // when type === 'array'
  itemChildren: SchemaField[] // when type === 'array' && itemType === 'object'
}

let counter = 0
const uid = () => `sf-${Date.now()}-${counter++}`

function emptyField(): SchemaField {
  return { id: uid(), name: '', type: 'string', required: false, children: [], itemType: 'string', itemChildren: [] }
}

// --- parse: JSON Schema string -> fields (null = not representable) --------

const OBJECT_KEYS = new Set(['type', 'properties', 'required', 'description', 'additionalProperties', 'title', '$schema'])
const FIELD_KEYS = new Set(['type', 'properties', 'required', 'description', 'items', 'additionalProperties', 'title'])

function parse(text: string): SchemaField[] | null {
  if (!text.trim()) return []
  let doc: unknown
  try {
    doc = JSON.parse(text)
  } catch {
    return null
  }
  return objectToFields(doc)
}

function objectToFields(node: unknown): SchemaField[] | null {
  if (!node || typeof node !== 'object' || Array.isArray(node)) return null
  const obj = node as Record<string, unknown>
  if (obj.type !== undefined && obj.type !== 'object') return null
  if (Object.keys(obj).some((k) => !OBJECT_KEYS.has(k))) return null

  const props = obj.properties && typeof obj.properties === 'object' ? (obj.properties as Record<string, unknown>) : {}
  const required = Array.isArray(obj.required) ? (obj.required as string[]) : []
  const out: SchemaField[] = []
  for (const [name, raw] of Object.entries(props)) {
    const f = rawToField(name, raw, required.includes(name))
    if (!f) return null
    out.push(f)
  }
  return out
}

function rawToField(name: string, raw: unknown, required: boolean): SchemaField | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null
  const r = raw as Record<string, unknown>
  if (Object.keys(r).some((k) => !FIELD_KEYS.has(k))) return null
  const base = { ...emptyField(), name, required }

  if (r.type === 'object') {
    const children = objectToFields(r)
    if (!children) return null
    return { ...base, type: 'object', children }
  }
  if (r.type === 'array') {
    const items = r.items && typeof r.items === 'object' ? (r.items as Record<string, unknown>) : {}
    if (items.type === 'object') {
      const itemChildren = objectToFields(items)
      if (!itemChildren) return null
      return { ...base, type: 'array', itemType: 'object', itemChildren }
    }
    if (items.type !== undefined && !PRIMS.includes(items.type as PrimType)) return null
    return { ...base, type: 'array', itemType: (items.type as PrimType) ?? 'string' }
  }
  if (PRIMS.includes(r.type as PrimType)) return { ...base, type: r.type as PrimType }
  return null
}

// --- serialize: fields -> JSON Schema string ("" when nothing defined) ----

function serialize(fields: SchemaField[]): string {
  if (!fields.some((f) => f.name.trim())) return ''
  return JSON.stringify(fieldsToObject(fields), null, 2)
}

function fieldsToObject(fields: SchemaField[]): Record<string, unknown> {
  const properties: Record<string, unknown> = {}
  const required: string[] = []
  for (const f of fields) {
    const name = f.name.trim()
    if (!name) continue
    properties[name] = fieldToObject(f)
    if (f.required) required.push(name)
  }
  const out: Record<string, unknown> = { type: 'object', properties }
  if (required.length) out.required = required
  return out
}

function fieldToObject(f: SchemaField): Record<string, unknown> {
  if (f.type === 'object') return fieldsToObject(f.children)
  if (f.type === 'array') {
    return { type: 'array', items: f.itemType === 'object' ? fieldsToObject(f.itemChildren) : { type: f.itemType } }
  }
  return { type: f.type }
}

// --- component -----------------------------------------------------------

interface SchemaBuilderProps {
  value: string
  onChange: (value: string) => void
}

export default function SchemaBuilder({ value, onChange }: SchemaBuilderProps) {
  const [fields, setFields] = useState<SchemaField[]>(() => parse(value) ?? [])
  const [mode, setMode] = useState<'builder' | 'raw'>(() => (value.trim() && parse(value) === null ? 'raw' : 'builder'))
  const [rawText, setRawText] = useState(value)

  const apply = (next: SchemaField[]) => {
    setFields(next)
    onChange(serialize(next))
  }

  if (mode === 'raw') {
    const parsed = parse(rawText)
    return (
      <div className="schema-builder">
        <div className="schema-builder-toolbar">
          <span className="hint">Raw JSON Schema</span>
          <button
            type="button"
            className="link-button"
            disabled={parsed === null}
            title={parsed === null ? 'This JSON can’t be shown in the builder — fix or clear it first' : undefined}
            onClick={() => {
              const p = parse(rawText)
              if (p) {
                setFields(p)
                setMode('builder')
              }
            }}
          >
            Use builder
          </button>
        </div>
        <LineNumberedTextarea
          rows={6}
          placeholder='{"type": "object", "properties": {"city": {"type": "string"}}}'
          value={rawText}
          onChange={(t) => {
            setRawText(t)
            onChange(t)
          }}
        />
      </div>
    )
  }

  return (
    <div className="schema-builder">
      <div className="schema-builder-toolbar">
        <button type="button" className="schema-add" onClick={() => apply([...fields, emptyField()])}>
          <Plus size={13} /> Add property
        </button>
        <button
          type="button"
          className="link-button"
          onClick={() => {
            setRawText(serialize(fields) || '{\n  "type": "object",\n  "properties": {}\n}')
            setMode('raw')
          }}
        >
          <Braces size={12} /> Edit raw JSON
        </button>
      </div>
      <FieldList fields={fields} onChange={apply} />
      {fields.length === 0 && <p className="hint">Add a property to start defining the shape.</p>}
    </div>
  )
}

interface FieldListProps {
  fields: SchemaField[]
  onChange: (fields: SchemaField[]) => void
}

function FieldList({ fields, onChange }: FieldListProps) {
  const set = (id: string, patch: Partial<SchemaField>) =>
    onChange(fields.map((f) => (f.id === id ? { ...f, ...patch } : f)))
  const remove = (id: string) => onChange(fields.filter((f) => f.id !== id))

  if (fields.length === 0) return null

  return (
    <ul className="schema-field-list">
      {fields.map((f) => (
        <li key={f.id} className="schema-field">
          <div className="schema-field-row">
            <input
              className="schema-field-name"
              placeholder="property"
              value={f.name}
              onChange={(e) => set(f.id, { name: e.target.value })}
            />
            <select value={f.type} onChange={(e) => set(f.id, { type: e.target.value as FieldType })}>
              <option value="string">string</option>
              <option value="number">number</option>
              <option value="integer">integer</option>
              <option value="boolean">boolean</option>
              <option value="object">object</option>
              <option value="array">array</option>
            </select>
            {f.type === 'array' && (
              <select
                value={f.itemType}
                onChange={(e) => set(f.id, { itemType: e.target.value as PrimType | 'object' })}
                aria-label="Array item type"
              >
                <option value="string">of string</option>
                <option value="number">of number</option>
                <option value="integer">of integer</option>
                <option value="boolean">of boolean</option>
                <option value="object">of object</option>
              </select>
            )}
            <label className="schema-field-req" title="Required">
              <input type="checkbox" checked={f.required} onChange={(e) => set(f.id, { required: e.target.checked })} />
              req
            </label>
            <button type="button" className="icon-button" aria-label="Remove property" onClick={() => remove(f.id)}>
              <Trash2 size={13} />
            </button>
          </div>

          {f.type === 'object' && (
            <div className="schema-field-children">
              <FieldList fields={f.children} onChange={(ch) => set(f.id, { children: ch })} />
              <button
                type="button"
                className="schema-add"
                onClick={() => set(f.id, { children: [...f.children, emptyField()] })}
              >
                <Plus size={12} /> Add property
              </button>
            </div>
          )}

          {f.type === 'array' && f.itemType === 'object' && (
            <div className="schema-field-children">
              <FieldList fields={f.itemChildren} onChange={(ch) => set(f.id, { itemChildren: ch })} />
              <button
                type="button"
                className="schema-add"
                onClick={() => set(f.id, { itemChildren: [...f.itemChildren, emptyField()] })}
              >
                <Plus size={12} /> Add item property
              </button>
            </div>
          )}
        </li>
      ))}
    </ul>
  )
}
