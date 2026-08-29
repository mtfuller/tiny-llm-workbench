import { Check, Search } from 'lucide-react'
import { useId, useMemo, useState } from 'react'
import type { Model } from './api'
import { Badge } from './Badge'
import { formatDate } from './lib/format'
import { modelSourceLabel } from './lib/models'
import Modal from './Modal'
import { suggestedModels } from './suggestedModels'

// A registry model saved before CreatedAt was tracked serializes as the Go
// zero time; don't render that as a date.
function createdLabel(iso?: string): string {
  return iso && !iso.startsWith('0001-01-01') ? formatDate(iso) : ''
}

// ModelCombobox is a free-text model picker: a text input backed by a
// <datalist> for quick autocomplete, plus a "Browse" button that opens a
// searchable modal listing the registry models with their metadata (source,
// base model, created date) so you don't have to remember exact names. Free
// text is still allowed — a raw "org/repo" Hugging Face id or a local path
// works anywhere a picked model does.
interface ModelComboboxProps {
  value: string
  onChange: (value: string) => void
  // The user's registry models. Names feed the datalist; the full objects
  // feed the browse modal's metadata rows.
  models: Model[]
  placeholder?: string
  autoFocus?: boolean
}

export default function ModelCombobox({
  value,
  onChange,
  models,
  placeholder = 'mlx-community/Qwen2.5-0.5B-Instruct-4bit',
  autoFocus,
}: ModelComboboxProps) {
  const listId = useId()
  const [browsing, setBrowsing] = useState(false)

  const options = useMemo(
    () => Array.from(new Set([...models.map((m) => m.name), ...suggestedModels])),
    [models],
  )

  return (
    <div className="model-combobox">
      <input
        type="text"
        list={listId}
        placeholder={placeholder}
        value={value}
        autoFocus={autoFocus}
        onChange={(e) => onChange(e.target.value)}
      />
      <button
        type="button"
        className="icon-button"
        title="Browse models"
        aria-label="Browse models"
        onClick={() => setBrowsing(true)}
      >
        <Search size={16} />
      </button>
      <datalist id={listId}>
        {options.map((name) => (
          <option key={name} value={name} />
        ))}
      </datalist>
      {browsing && (
        <ModelPickerModal
          models={models}
          current={value}
          onPick={(name) => {
            onChange(name)
            setBrowsing(false)
          }}
          onClose={() => setBrowsing(false)}
        />
      )}
    </div>
  )
}

interface ModelPickerModalProps {
  models: Model[]
  current: string
  onPick: (name: string) => void
  onClose: () => void
}

function ModelPickerModal({ models, current, onPick, onClose }: ModelPickerModalProps) {
  const [query, setQuery] = useState('')
  const q = query.trim().toLowerCase()

  const registryNames = useMemo(() => new Set(models.map((m) => m.name)), [models])
  const suggestions = suggestedModels.filter((s) => !registryNames.has(s))

  const yours = models.filter(
    (m) => !q || `${m.name} ${m.baseModel ?? ''} ${modelSourceLabel(m)}`.toLowerCase().includes(q),
  )
  const suggested = suggestions.filter((s) => !q || s.toLowerCase().includes(q))

  return (
    <Modal title="Choose a model" onClose={onClose} size="lg">
      <input
        type="search"
        className="hf-search-input"
        placeholder="Filter by name, base model, or source…"
        value={query}
        autoFocus
        onChange={(e) => setQuery(e.target.value)}
      />

      <div className="model-picker-list">
        {yours.length > 0 && <div className="model-picker-group">Your models</div>}
        {yours.map((m) => (
          <button
            key={m.name}
            type="button"
            className={`model-picker-row${m.name === current ? ' is-current' : ''}`}
            onClick={() => onPick(m.name)}
          >
            <span className="model-picker-row-main">
              <span className="model-picker-row-name">
                {m.name}
                {m.name === current && <Check size={14} />}
              </span>
              {(m.baseModel || createdLabel(m.createdAt)) && (
                <span className="model-picker-row-sub">
                  {[m.baseModel ? `from ${m.baseModel}` : '', createdLabel(m.createdAt)].filter(Boolean).join(' · ')}
                </span>
              )}
            </span>
            <Badge>{modelSourceLabel(m)}</Badge>
          </button>
        ))}

        {suggested.length > 0 && <div className="model-picker-group">Suggested — downloaded on first use</div>}
        {suggested.map((s) => (
          <button
            key={s}
            type="button"
            className={`model-picker-row${s === current ? ' is-current' : ''}`}
            onClick={() => onPick(s)}
          >
            <span className="model-picker-row-main">
              <span className="model-picker-row-name">
                {s}
                {s === current && <Check size={14} />}
              </span>
            </span>
            <Badge>Hugging Face</Badge>
          </button>
        ))}

        {yours.length === 0 && suggested.length === 0 && (
          <p className="hint">
            No models match “{query}”. You can still type any Hugging Face repo id (<code>org/name</code>) or a
            local path directly into the field.
          </p>
        )}
      </div>
    </Modal>
  )
}
