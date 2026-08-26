import { useState, type KeyboardEvent } from 'react'

interface TagInputProps {
  tags: string[]
  onChange: (tags: string[]) => void
  suggestions?: string[]
  id?: string
}

// TagInput is a small chips-plus-text-field editor: type a tag and press
// Enter/comma (or blur the field) to commit it, click a chip's × to remove
// it. suggestions (typically every tag already used elsewhere in the
// dataset) populate a datalist so tags stay consistent without a separate
// tag-management screen.
function TagInput({ tags, onChange, suggestions = [], id = 'tag-input' }: TagInputProps) {
  const [draft, setDraft] = useState('')

  const commit = (raw: string) => {
    const value = raw.trim()
    setDraft('')
    if (!value || tags.includes(value)) return
    onChange([...tags, value])
  }

  const removeTag = (tag: string) => onChange(tags.filter((t) => t !== tag))

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      commit(draft)
    } else if (e.key === 'Backspace' && draft === '' && tags.length > 0) {
      removeTag(tags[tags.length - 1])
    }
  }

  const suggestionsListId = `${id}-suggestions`

  return (
    <div className="tag-input">
      {tags.map((tag) => (
        <span className="tag-chip" key={tag}>
          {tag}
          <button type="button" onClick={() => removeTag(tag)} aria-label={`Remove tag ${tag}`}>
            ×
          </button>
        </span>
      ))}
      <input
        type="text"
        list={suggestionsListId}
        className="tag-input-field"
        placeholder={tags.length === 0 ? 'Add a tag…' : ''}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={() => commit(draft)}
      />
      <datalist id={suggestionsListId}>
        {suggestions
          .filter((s) => !tags.includes(s))
          .map((s) => (
            <option key={s} value={s} />
          ))}
      </datalist>
    </div>
  )
}

export default TagInput
