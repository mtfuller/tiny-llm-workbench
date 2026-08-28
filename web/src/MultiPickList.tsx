// MultiPickList is a checkbox list of named catalog items (tools or
// knowledge bases) with an optional one-line description each — used by the
// Agent settings modal and the agent node's tool/knowledge selectors.
// Mirrors the tool node's .tool-arg-row styling so all three read as the
// same component family.
export default function MultiPickList({
  options,
  selected,
  onToggle,
  emptyMessage,
}: {
  options: { name: string; description?: string }[]
  selected: string[]
  onToggle: (name: string, checked: boolean) => void
  emptyMessage: string
}) {
  if (options.length === 0) return <p className="hint">{emptyMessage}</p>
  return (
    <div className="tool-pick-list">
      {options.map((o) => (
        <label key={o.name} className="tool-pick-row">
          <input type="checkbox" checked={selected.includes(o.name)} onChange={(e) => onToggle(o.name, e.target.checked)} />
          <span className="tool-pick-row-body">
            <span className="tool-pick-row-name">{o.name}</span>
            {o.description && <span className="field-hint">{o.description}</span>}
          </span>
        </label>
      ))}
    </div>
  )
}
