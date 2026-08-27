import { FileJson, FileSpreadsheet, Pencil, Plus, Sparkles, Trash2, Upload } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  addExamples,
  datasetExportUrl,
  deleteExample,
  generateVariations,
  getDataset,
  importDataset,
  listModels,
  updateExample,
  type DatasetDetail as DatasetDetailType,
  type Example,
  type Model,
} from '../api'
import { useConfirm } from '../ConfirmDialog'
import LineNumberedTextarea from '../LineNumberedTextarea'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { suggestedModels } from '../suggestedModels'
import TagFilterDropdown from '../TagFilterDropdown'
import TagInput from '../TagInput'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'

const emptyExample: Example = { input: '', output: '', description: '', tags: [] }

type ModalState = { mode: 'add' } | { mode: 'edit'; index: number } | null

function DatasetDetail() {
  const confirm = useConfirm()
  const showToast = useToast()
  const { name = '' } = useParams<{ name: string }>()
  const [dataset, setDataset] = useState<DatasetDetailType | null>(null)
  const [models, setModels] = useState<Model[]>([])
  const [error, setError] = useState<string | null>(null)

  const [search, setSearch] = useState('')
  const [activeTags, setActiveTags] = useState<Set<string>>(new Set())

  const [modal, setModal] = useState<ModalState>(null)
  const [modalSaving, setModalSaving] = useState(false)
  const [modalError, setModalError] = useState<string | null>(null)
  const [deletingIndex, setDeletingIndex] = useState<number | null>(null)

  const [generateOpen, setGenerateOpen] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [generateError, setGenerateError] = useState<string | null>(null)

  const [importing, setImporting] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const reload = () => {
    getDataset(name)
      .then(setDataset)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(reload, [name])
  useEffect(() => {
    listModels()
      .then(setModels)
      .catch(() => setModels([]))
  }, [])

  const modelOptions = useMemo(() => {
    const trained = models.map((m) => m.name)
    return Array.from(new Set([...trained, ...suggestedModels]))
  }, [models])

  const examples = dataset?.examples ?? []

  const allTags = useMemo(() => {
    const tags = new Set<string>()
    for (const ex of examples) {
      for (const t of ex.tags ?? []) tags.add(t)
    }
    return Array.from(tags).sort()
  }, [examples])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return examples
      .map((example, index) => ({ example, index }))
      .filter(({ example }) => {
        if (activeTags.size > 0) {
          const tags = example.tags ?? []
          if (![...activeTags].some((t) => tags.includes(t))) return false
        }
        if (q) {
          const haystack = `${example.input} ${example.output} ${example.description ?? ''}`.toLowerCase()
          if (!haystack.includes(q)) return false
        }
        return true
      })
  }, [examples, search, activeTags])

  const { page: currentPage, setPage, resetPage, pageCount, pageItems } = usePagination(filtered)

  useEffect(resetPage, [search, activeTags, resetPage])

  const toggleTagFilter = (tag: string) => {
    setActiveTags((prev) => {
      const next = new Set(prev)
      if (next.has(tag)) next.delete(tag)
      else next.add(tag)
      return next
    })
  }

  const handleSaveModal = async (example: Example) => {
    if (!modal) return

    setModalSaving(true)
    setModalError(null)
    try {
      if (modal.mode === 'add') {
        await addExamples(name, [example])
        showToast('Added example')
      } else {
        await updateExample(name, modal.index, example)
        showToast('Saved example')
      }
      setModal(null)
      reload()
    } catch (err) {
      setModalError((err as Error).message)
    } finally {
      setModalSaving(false)
    }
  }

  const handleDeleteExample = async (index: number) => {
    if (!(await confirm('Delete this example? This cannot be undone.'))) return

    setDeletingIndex(index)
    setError(null)
    try {
      await deleteExample(name, index)
      showToast('Deleted example')
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setDeletingIndex(null)
    }
  }

  const handleGenerate = async (req: { model: string; seed: Example; count: number }) => {
    setGenerating(true)
    setGenerateError(null)
    try {
      await generateVariations(name, req)
      showToast(`Generated ${req.count} variation${req.count === 1 ? '' : 's'}`)
      setGenerateOpen(false)
      reload()
    } catch (err) {
      setGenerateError((err as Error).message)
    } finally {
      setGenerating(false)
    }
  }

  const handleImportFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

    const format = file.name.toLowerCase().endsWith('.csv') ? 'csv' : 'jsonl'
    setImporting(true)
    setError(null)
    try {
      const content = await file.text()
      await importDataset(name, format, content)
      showToast('Imported examples')
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setImporting(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  return (
    <>
      <div className="page-header">
        <h2>
          <Link to="/datasets">Datasets</Link> / {name}
        </h2>
      </div>
      {dataset?.title && <h3 className="dataset-title">{dataset.title}</h3>}
      {dataset?.description && <p className="hint">{dataset.description}</p>}

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search examples…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
          {allTags.length > 0 && (
            <TagFilterDropdown tags={allTags} active={activeTags} onToggle={toggleTagFilter} onClear={() => setActiveTags(new Set())} />
          )}
          <div className="list-toolbar-actions">
            <button
              type="button"
              className="icon-button"
              title="Add example"
              aria-label="Add example"
              onClick={() => setModal({ mode: 'add' })}
            >
              <Plus size={16} />
            </button>
            <button
              type="button"
              className="icon-button"
              title="Generate variations…"
              aria-label="Generate variations"
              onClick={() => setGenerateOpen(true)}
            >
              <Sparkles size={16} />
            </button>
            <label
              className="icon-button"
              title={
                importing
                  ? 'Importing…'
                  : 'Import CSV or JSONL — CSV needs "input" and "output" columns (any order); optional ' +
                    '"description" and "tags" columns are picked up too, tags semicolon-separated.'
              }
              aria-label="Import examples"
            >
              <Upload size={16} />
              <input
                ref={fileInputRef}
                type="file"
                accept=".csv,.jsonl,.txt"
                onChange={handleImportFile}
                disabled={importing}
                hidden
              />
            </label>
            <a className="icon-button" title="Export JSONL" aria-label="Export JSONL" href={datasetExportUrl(name, 'jsonl')}>
              <FileJson size={16} />
            </a>
            <a className="icon-button" title="Export CSV" aria-label="Export CSV" href={datasetExportUrl(name, 'csv')}>
              <FileSpreadsheet size={16} />
            </a>
          </div>
        </div>

        {error && (
          <div className="panel-body">
            <p className="error">{error}</p>
          </div>
        )}

        {!error && dataset === null && (
          <div className="panel-body">
            <TableSkeleton columns={5} />
          </div>
        )}

        {dataset !== null && examples.length === 0 && (
          <div className="panel-body">
            <p className="hint">No examples yet. Add one above or generate some.</p>
          </div>
        )}

        {dataset !== null && examples.length > 0 && filtered.length === 0 && (
          <div className="panel-body">
            <p className="hint">No examples match your search/filter.</p>
          </div>
        )}

        {filtered.length > 0 && (
          <table className="data-table">
            <thead>
              <tr>
                <th>Input</th>
                <th>Output</th>
                <th>Description</th>
                <th>Tags</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {pageItems.map(({ example, index }) => (
                <tr key={index}>
                  <td className="cell-truncate">{example.input}</td>
                  <td className="cell-truncate">{example.output}</td>
                  <td className="cell-truncate">{example.description || '—'}</td>
                  <td>
                    {example.tags && example.tags.length > 0 ? (
                      <div className="tag-list">
                        {example.tags.map((tag) => (
                          <span className="badge" key={tag}>
                            {tag}
                          </span>
                        ))}
                      </div>
                    ) : (
                      '—'
                    )}
                  </td>
                  <td className="row-actions">
                    <button
                      type="button"
                      className="icon-button"
                      aria-label="Edit example"
                      onClick={() => setModal({ mode: 'edit', index })}
                    >
                      <Pencil size={15} />
                    </button>
                    <button
                      type="button"
                      className="icon-button"
                      aria-label="Delete example"
                      disabled={deletingIndex === index}
                      onClick={() => handleDeleteExample(index)}
                    >
                      <Trash2 size={15} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        <Pagination
          page={currentPage}
          pageCount={pageCount}
          onChange={setPage}
          shownCount={filtered.length}
          totalCount={examples.length}
          itemLabel="examples"
        />
      </div>

      {modal && (
        <ExampleModal
          title={modal.mode === 'add' ? 'Add example' : 'Edit example'}
          initial={modal.mode === 'edit' ? examples[modal.index] : emptyExample}
          allTags={allTags}
          saving={modalSaving}
          error={modalError}
          onSave={handleSaveModal}
          onClose={() => setModal(null)}
        />
      )}

      {generateOpen && (
        <GenerateVariationsModal
          modelOptions={modelOptions}
          generating={generating}
          error={generateError}
          onGenerate={handleGenerate}
          onClose={() => setGenerateOpen(false)}
        />
      )}
    </>
  )
}

interface ExampleModalProps {
  title: string
  initial: Example
  allTags: string[]
  saving: boolean
  error: string | null
  onSave: (example: Example) => void
  onClose: () => void
}

// ExampleModal is the single input/output/description/tags editing surface
// used for both adding a new example and editing an existing one.
function ExampleModal({ title, initial, allTags, saving, error, onSave, onClose }: ExampleModalProps) {
  const [input, setInput] = useState(initial.input)
  const [output, setOutput] = useState(initial.output)
  const [description, setDescription] = useState(initial.description ?? '')
  const [tags, setTags] = useState<string[]>(initial.tags ?? [])

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!input.trim() || !output.trim()) return
    onSave({
      input: input.trim(),
      output: output.trim(),
      description: description.trim() || undefined,
      tags: tags.length > 0 ? tags : undefined,
    })
  }

  return (
    <Modal title={title} onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Input
          <LineNumberedTextarea value={input} onChange={setInput} rows={5} autoFocus />
        </label>
        <label>
          Output
          <LineNumberedTextarea value={output} onChange={setOutput} rows={5} />
        </label>
        <label>
          Description (optional)
          <input
            type="text"
            placeholder="A short note about this example"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </label>
        <label>
          Tags
          <TagInput tags={tags} onChange={setTags} suggestions={allTags} />
        </label>
        {error && <p className="error">{error}</p>}
        <div className="row-actions confirm-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={saving || !input.trim() || !output.trim()}>
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

interface GenerateVariationsModalProps {
  modelOptions: string[]
  generating: boolean
  error: string | null
  onGenerate: (req: { model: string; seed: Example; count: number }) => void
  onClose: () => void
}

function GenerateVariationsModal({ modelOptions, generating, error, onGenerate, onClose }: GenerateVariationsModalProps) {
  const [model, setModel] = useState('')
  const [seedInput, setSeedInput] = useState('')
  const [seedOutput, setSeedOutput] = useState('')
  const [count, setCount] = useState(3)

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!model || !seedInput.trim() || !seedOutput.trim()) return
    onGenerate({ model, seed: { input: seedInput.trim(), output: seedOutput.trim() }, count })
  }

  return (
    <Modal title="Generate variations" onClose={onClose}>
      <p className="hint">Give one example input/output pair and a local model will generate similar ones.</p>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Model
          <input
            type="text"
            list="generate-model-options"
            placeholder="mlx-community/Qwen2.5-0.5B-Instruct-4bit"
            value={model}
            onChange={(e) => setModel(e.target.value)}
          />
          <datalist id="generate-model-options">
            {modelOptions.map((name) => (
              <option key={name} value={name} />
            ))}
          </datalist>
        </label>
        <label>
          Example input
          <textarea rows={2} value={seedInput} onChange={(e) => setSeedInput(e.target.value)} />
        </label>
        <label>
          Example output
          <textarea rows={2} value={seedOutput} onChange={(e) => setSeedOutput(e.target.value)} />
        </label>
        <label>
          How many variations
          <input type="number" min={1} max={20} value={count} onChange={(e) => setCount(Number(e.target.value))} />
        </label>
        {error && <p className="error">{error}</p>}
        <div className="row-actions confirm-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={generating || !model}>
            {generating ? 'Generating…' : 'Generate'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

export default DatasetDetail
