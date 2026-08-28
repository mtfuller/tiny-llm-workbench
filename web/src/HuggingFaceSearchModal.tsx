import { Check, Download, ExternalLink } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { addHuggingFaceModel, searchHuggingFaceModels, type HuggingFaceModel } from './api'
import Modal from './Modal'

interface Props {
  // Called after a model is added so the caller can refresh its list.
  onAdded: () => void
  onClose: () => void
}

// Tags worth surfacing on a result row — quantization and the task, mostly.
// The full tag list is noisy (licence, region, library, …).
const INTERESTING_TAGS = /^(\d+-bit|\d+bit|text-generation|conversational|vision|code|instruct|chat)$/i

function HuggingFaceSearchModal({ onAdded, onClose }: Props) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<HuggingFaceModel[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [adding, setAdding] = useState<string | null>(null)
  // Repos added during this modal session (so the row updates without a refetch).
  const [addedRepos, setAddedRepos] = useState<Set<string>>(new Set())
  const reqId = useRef(0)

  // Debounced search — also runs once on mount with an empty query (most
  // downloaded mlx-community models).
  useEffect(() => {
    const id = ++reqId.current
    setLoading(true)
    setError(null)
    const t = setTimeout(
      () => {
        searchHuggingFaceModels(query)
          .then((r) => {
            if (id === reqId.current) setResults(r)
          })
          .catch((e: Error) => {
            if (id === reqId.current) setError(e.message)
          })
          .finally(() => {
            if (id === reqId.current) setLoading(false)
          })
      },
      query ? 300 : 0,
    )
    return () => clearTimeout(t)
  }, [query])

  const handleAdd = async (m: HuggingFaceModel) => {
    setAdding(m.repoId)
    setError(null)
    try {
      await addHuggingFaceModel(m.repoId)
      setAddedRepos((prev) => new Set(prev).add(m.repoId))
      onAdded()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setAdding(null)
    }
  }

  return (
    <Modal title="Add a model from Hugging Face" onClose={onClose} size="lg">
      <p className="hint">
        Search the <code>mlx-community</code> org for MLX-ready small models. Added models download
        automatically the first time they're used.
      </p>

      <input
        type="text"
        autoFocus
        placeholder="Search models (e.g. qwen 0.5b, llama 3.2, phi)…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        className="hf-search-input"
      />

      {error && <p className="error">{error}</p>}

      <div className="hf-results">
        {loading && <p className="hint">Searching…</p>}
        {!loading && results.length === 0 && <p className="hint">No mlx-community models match that search.</p>}
        {!loading &&
          results.map((m) => {
            const added = m.added || addedRepos.has(m.repoId)
            const tags = m.tags.filter((t) => INTERESTING_TAGS.test(t)).slice(0, 4)
            return (
              <div className="hf-result-row" key={m.repoId}>
                <div className="hf-result-main">
                  <div className="hf-result-name">
                    {m.name}
                    <a
                      href={`https://huggingface.co/${m.repoId}`}
                      target="_blank"
                      rel="noreferrer"
                      className="hf-result-link"
                      title="View on Hugging Face"
                    >
                      <ExternalLink size={12} />
                    </a>
                  </div>
                  <div className="hf-result-meta">
                    {m.downloads.toLocaleString()} downloads · {m.likes} likes
                    {tags.length > 0 && <span className="hf-result-tags"> · {tags.join(', ')}</span>}
                  </div>
                </div>
                {added ? (
                  <span className="hf-result-added">
                    <Check size={14} /> Added
                  </span>
                ) : (
                  <button
                    type="button"
                    className="hf-add-button"
                    disabled={adding === m.repoId}
                    onClick={() => handleAdd(m)}
                  >
                    <Download size={13} /> {adding === m.repoId ? 'Adding…' : 'Add'}
                  </button>
                )}
              </div>
            )
          })}
      </div>
    </Modal>
  )
}

export default HuggingFaceSearchModal
