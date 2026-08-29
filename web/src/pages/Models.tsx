import { Download, Play, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { deleteModel, listModels, type Model } from '../api'
import HuggingFaceSearchModal from '../HuggingFaceSearchModal'
import IconButton from '../IconButton'
import { modelSourceLabel } from '../lib/models'
import ListPanel from '../ListPanel'
import ModelChatModal from '../ModelChatModal'
import { useResourceList } from '../useResourceList'

function Models() {
  const list = useResourceList<Model>({
    load: listModels,
    getName: (m) => m.name,
    searchText: (m) => m.baseModel ?? '',
    remove: (m) => deleteModel(m.name),
    confirmMessage: (m) => `Delete model "${m.name}"? This cannot be undone.`,
    deletedToast: (m) => `Deleted model "${m.name}"`,
  })
  const [chatModel, setChatModel] = useState<string | null>(null)
  const [hfOpen, setHfOpen] = useState(false)

  return (
    <>
      <div className="page-header">
        <h2>Models</h2>
      </div>
      <p className="hint">
        Models trained in TLW, plus any pulled from Hugging Face. A repo id (e.g.{' '}
        <code>mlx-community/Qwen2.5-0.5B-Instruct-4bit</code>) can also be typed straight into any model
        picker — it's downloaded automatically on first use.
      </p>

      <ListPanel
        search={list.search}
        onSearch={list.setSearch}
        searchPlaceholder="Search models…"
        actions={
          <IconButton icon={<Download size={16} />} label="Add from Hugging Face" onClick={() => setHfOpen(true)} />
        }
        error={list.error && `Failed to load models: ${list.error}`}
        loading={list.items === null}
        isEmpty={list.items !== null && list.items.length === 0}
        hasMatches={list.filtered.length > 0}
        emptyMessage="No models yet. Train one on the Training page to get started."
        noMatchMessage="No models match your search."
        skeletonColumns={4}
        page={list.page}
        pageCount={list.pageCount}
        setPage={list.setPage}
        shownCount={list.filtered.length}
        totalCount={list.items?.length ?? 0}
        itemLabel="models"
      >
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Source</th>
              <th>Base model</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.pageItems.map((model) => (
              <tr key={model.name}>
                <td>
                  <Link to={`/models/${encodeURIComponent(model.name)}`}>{model.name}</Link>
                </td>
                <td>{modelSourceLabel(model)}</td>
                <td>{model.baseModel || '—'}</td>
                <td className="row-actions">
                  <IconButton icon={<Play size={15} />} label="Run / prompt model" onClick={() => setChatModel(model.name)} />
                  <IconButton
                    icon={<Trash2 size={15} />}
                    label="Delete model"
                    disabled={list.deleting === model.name}
                    onClick={() => list.handleDelete(model)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ListPanel>

      {chatModel && <ModelChatModal modelName={chatModel} onClose={() => setChatModel(null)} />}
      {hfOpen && <HuggingFaceSearchModal onAdded={list.reload} onClose={() => setHfOpen(false)} />}
    </>
  )
}

export default Models
