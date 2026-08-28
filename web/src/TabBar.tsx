// TabBar renders the minimal segmented tab strip (`.tab-bar` / `.tab-button`)
// used on the versioned detail pages and the Environment workspace.
interface TabBarProps<T extends string> {
  tabs: readonly { value: T; label: string }[]
  value: T
  onChange: (value: T) => void
}

export default function TabBar<T extends string>({ tabs, value, onChange }: TabBarProps<T>) {
  return (
    <div className="tab-bar">
      {tabs.map((tab) => (
        <button
          key={tab.value}
          type="button"
          className={`tab-button${value === tab.value ? ' tab-button-active' : ''}`}
          onClick={() => onChange(tab.value)}
        >
          {tab.label}
        </button>
      ))}
    </div>
  )
}
