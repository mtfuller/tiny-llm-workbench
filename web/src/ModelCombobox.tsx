import { useId, useMemo } from 'react'
import { suggestedModels } from './suggestedModels'

// ModelCombobox is a free-text model picker backed by a <datalist> of the
// user's trained models plus the well-known suggested repo ids. Every call
// site previously built its own `modelOptions` memo and hand-picked a
// `<datalist id>` (AgentEditor even reused one id twice). `useId` gives each
// instance a unique, stable list id.
interface ModelComboboxProps {
  value: string
  onChange: (value: string) => void
  // The user's trained model names; suggested repo ids are appended.
  models: string[]
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
  const options = useMemo(() => Array.from(new Set([...models, ...suggestedModels])), [models])

  return (
    <>
      <input
        type="text"
        list={listId}
        placeholder={placeholder}
        value={value}
        autoFocus={autoFocus}
        onChange={(e) => onChange(e.target.value)}
      />
      <datalist id={listId}>
        {options.map((name) => (
          <option key={name} value={name} />
        ))}
      </datalist>
    </>
  )
}
