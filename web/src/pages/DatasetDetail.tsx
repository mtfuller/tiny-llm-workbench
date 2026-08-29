import {
  AlertTriangle,
  Bot,
  Check,
  Copy,
  FileJson,
  FileSpreadsheet,
  Flag,
  Pencil,
  Plus,
  Sparkles,
  Trash2,
  Upload,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  addExamples,
  approveExample,
  datasetExportUrl,
  deleteExample,
  flagExampleForReview,
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
import FilterMenu from '../FilterMenu'
import IconButton from '../IconButton'
import LineNumberedTextarea from '../LineNumberedTextarea'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import ModelCombobox from '../ModelCombobox'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import TagCell from '../TagCell'
import TagInput from '../TagInput'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'
import { useTagFilter } from '../useTagFilter'

const emptyExample: Example = { input: '', output: '', description: '', tags: [] }

const REVIEW_FILTER = 'Needs review'

// exampleNeedsReview is true when a human should look at a record before it's
// trusted for training: either an AI-generated pair nobody has approved yet,
// or one a human explicitly flagged.
function exampleNeedsReview(ex: Example): boolean {
  return ex.needsReview === true || (ex.source === 'ai' && !ex.approved)
}

type ModalState = { mode: 'add' } | { mode: 'edit'; index: number } | null

function DatasetDetail() {
  const confirm = useConfirm()
  const showToast = useToast()
  const { name = '' } = useParams<{ name: string }>()
  const [dataset, setDataset] = useState<DatasetDetailType | null>(null)
  const [models, setModels] = useState<Model[]>([])
  const [error, setError] = useState<string | null>(null)

  const [search, setSearch] = useState('')

  const [modal, setModal] = useState<ModalState>(null)
  const [modalSaving, setModalSaving] = useState(false)
  const [modalError, setModalError] = useState<string | null>(null)
  const [deletingIndex, setDeletingIndex] = useState<number | null>(null)
  const [approvingIndex, setApprovingIndex] = useState<number | null>(null)
  const [flaggingIndex, setFlaggingIndex] = useState<number | null>(null)
  const [duplicatingIndex, setDuplicatingIndex] = useState<number | null>(null)

  const [reviewOnly, setReviewOnly] = useState(false)

  // null = closed; an object opens the modal, optionally pre-seeded from an
  // existing row's input/output ("generate variations from this record").
  const [generateState, setGenerateState] = useState<{ seed?: Example } | null>(null)
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

  const examples = dataset?.examples ?? []

  const needsReviewCount = useMemo(() => examples.filter(exampleNeedsReview).length, [examples])

  const { allTags, activeTags, toggleTag, clearTags, matchesTags } = useTagFilter(examples, (ex) => ex.tags)

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return examples
      .map((example, index) => ({ example, index }))
      .filter(({ example }) => {
        if (!matchesTags(example)) return false
        if (reviewOnly && !exampleNeedsReview(example)) return false
        if (q) {
          const haystack = `${example.input} ${example.output} ${example.description ?? ''}`.toLowerCase()
          if (!haystack.includes(q)) return false
        }
        return true
      })
  }, [examples, search, matchesTags, reviewOnly])

  const { page: currentPage, setPage, resetPage, pageCount, pageItems } = usePagination(filtered)

  useEffect(resetPage, [search, activeTags, reviewOnly, resetPage])

  const clearAllFilters = () => {
    clearTags()
    setReviewOnly(false)
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

  const handleApproveExample = async (index: number) => {
    setApprovingIndex(index)
    setError(null)
    try {
      await approveExample(name, index)
      showToast('Marked example as reviewed')
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setApprovingIndex(null)
    }
  }

  const handleFlagExample = async (index: number) => {
    setFlaggingIndex(index)
    setError(null)
    try {
      await flagExampleForReview(name, index)
      showToast('Flagged example for review')
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setFlaggingIndex(null)
    }
  }

  const handleDuplicateExample = async (example: Example, index: number) => {
    setDuplicatingIndex(index)
    setError(null)
    try {
      // A copy hasn't been individually reviewed — carry the content and
      // provenance, but let it re-enter the review queue.
      await addExamples(name, [{ ...example, approved: false, needsReview: false }])
      showToast('Duplicated example')
      reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setDuplicatingIndex(null)
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
      showToast(`Generated ${req.count} variation${req.count === 1 ? '' : 's'} — review before training`)
      setGenerateState(null)
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

      {needsReviewCount > 0 && (
        <div className="dataset-warning-banner">
          <AlertTriangle size={16} />
          <span>
            {needsReviewCount} {needsReviewCount === 1 ? 'example needs' : 'examples need'} human review (AI-generated or
            flagged). Check {needsReviewCount === 1 ? 'it' : 'them'} before training on this dataset.
          </span>
          {!reviewOnly && (
            <button type="button" className="dataset-warning-banner-action" onClick={() => setReviewOnly(true)}>
              Show only these
            </button>
          )}
        </div>
      )}

      <div className="panel panel-flush">
        <div className="list-toolbar panel-toolbar">
          <input
            type="search"
            placeholder="Search examples…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="list-search"
          />
          <FilterMenu
            groups={[
              {
                key: 'review',
                title: 'Review',
                options: [REVIEW_FILTER],
                active: reviewOnly ? new Set([REVIEW_FILTER]) : new Set(),
                onToggle: () => setReviewOnly((v) => !v),
              },
              { key: 'tags', title: 'Tags', options: allTags, active: activeTags, onToggle: toggleTag },
            ]}
            onClearAll={clearAllFilters}
          />
          <div className="list-toolbar-actions">
            <IconButton icon={<Plus size={16} />} label="Add example" onClick={() => setModal({ mode: 'add' })} />
            <IconButton icon={<Sparkles size={16} />} label="Generate variations…" onClick={() => setGenerateState({})} />
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
            <TableSkeleton columns={6} />
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
                <th>Source</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {pageItems.map(({ example, index }) => {
                const isAI = example.source === 'ai'
                const needsReview = exampleNeedsReview(example)
                return (
                  <tr key={index}>
                    <td className="cell-truncate">{example.input}</td>
                    <td className="cell-truncate">{example.output}</td>
                    <td className="cell-truncate">{example.description || '—'}</td>
                    <td>
                      <TagCell tags={example.tags} />
                    </td>
                    <td>
                      {needsReview ? (
                        <span
                          className="example-flag example-flag-warn"
                          title={
                            example.needsReview
                              ? 'Flagged by a human for review'
                              : 'Generated by AI — not yet human-reviewed'
                          }
                        >
                          <AlertTriangle size={12} /> {example.needsReview ? 'Needs review' : 'Unreviewed AI'}
                        </span>
                      ) : isAI ? (
                        <span className="example-flag example-flag-ai" title="Generated by AI, human-reviewed">
                          <Bot size={12} /> AI · reviewed
                        </span>
                      ) : (
                        '—'
                      )}
                    </td>
                    <td className="row-actions">
                      {needsReview ? (
                        <IconButton
                          icon={<Check size={15} />}
                          label="Mark as reviewed"
                          disabled={approvingIndex === index}
                          onClick={() => handleApproveExample(index)}
                        />
                      ) : (
                        <IconButton
                          icon={<Flag size={15} />}
                          label="Flag for review"
                          disabled={flaggingIndex === index}
                          onClick={() => handleFlagExample(index)}
                        />
                      )}
                      <IconButton
                        icon={<Copy size={15} />}
                        label="Duplicate example"
                        disabled={duplicatingIndex === index}
                        onClick={() => handleDuplicateExample(example, index)}
                      />
                      <IconButton
                        icon={<Sparkles size={15} />}
                        label="Generate variations from this record"
                        onClick={() => setGenerateState({ seed: example })}
                      />
                      <IconButton icon={<Pencil size={15} />} label="Edit example" onClick={() => setModal({ mode: 'edit', index })} />
                      <IconButton
                        icon={<Trash2 size={15} />}
                        label="Delete example"
                        disabled={deletingIndex === index}
                        onClick={() => handleDeleteExample(index)}
                      />
                    </td>
                  </tr>
                )
              })}
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

      {generateState && (
        <GenerateVariationsModal
          models={models}
          seed={generateState.seed}
          generating={generating}
          error={generateError}
          onGenerate={handleGenerate}
          onClose={() => setGenerateState(null)}
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

  const wasNeedingReview = exampleNeedsReview(initial)

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!input.trim() || !output.trim()) return
    onSave({
      input: input.trim(),
      output: output.trim(),
      description: description.trim() || undefined,
      tags: tags.length > 0 ? tags : undefined,
      // Keep the AI-provenance marker, but treat a human editing and saving
      // it as a review — any "needs review" flag clears.
      source: initial.source || undefined,
      approved: initial.source === 'ai' ? true : initial.approved,
      needsReview: false,
    })
  }

  return (
    <Modal title={title} onClose={onClose} size="xl">
      <form className="stacked-form example-form" onSubmit={handleSubmit}>
        {wasNeedingReview && (
          <p className="hint">
            <AlertTriangle size={13} /> This example is flagged for review. Saving it here clears that flag.
          </p>
        )}
        <div className="example-editor-fields">
          <label>
            Input
            <LineNumberedTextarea value={input} onChange={setInput} rows={16} autoFocus />
          </label>
          <label>
            Output
            <LineNumberedTextarea value={output} onChange={setOutput} rows={16} />
          </label>
        </div>
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
        <ModalActions onCancel={onClose} busy={saving} disabled={!input.trim() || !output.trim()} />
      </form>
    </Modal>
  )
}

interface GenerateVariationsModalProps {
  models: Model[]
  seed?: Example
  generating: boolean
  error: string | null
  onGenerate: (req: { model: string; seed: Example; count: number }) => void
  onClose: () => void
}

function GenerateVariationsModal({ models, seed, generating, error, onGenerate, onClose }: GenerateVariationsModalProps) {
  const [model, setModel] = useState('')
  const [seedInput, setSeedInput] = useState(seed?.input ?? '')
  const [seedOutput, setSeedOutput] = useState(seed?.output ?? '')
  const [count, setCount] = useState(3)

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!model || !seedInput.trim() || !seedOutput.trim()) return
    onGenerate({ model, seed: { input: seedInput.trim(), output: seedOutput.trim() }, count })
  }

  return (
    <Modal title="Generate variations" onClose={onClose} size="xl">
      <p className="hint">
        {seed
          ? 'Generating from the selected record — a local model will produce similar input/output pairs. Tweak the seed below if you like.'
          : 'Give one example input/output pair and a local model will generate similar ones.'}
      </p>
      <p className="hint">
        <AlertTriangle size={13} /> Generated pairs are flagged as unreviewed AI until you approve them.
      </p>
      <form className="stacked-form example-form" onSubmit={handleSubmit}>
        <label>
          Model
          <ModelCombobox value={model} onChange={setModel} models={models} />
        </label>
        <div className="example-editor-fields">
          <label>
            Example input
            <LineNumberedTextarea value={seedInput} onChange={setSeedInput} rows={14} />
          </label>
          <label>
            Example output
            <LineNumberedTextarea value={seedOutput} onChange={setSeedOutput} rows={14} />
          </label>
        </div>
        <label>
          How many variations
          <input type="number" min={1} max={20} value={count} onChange={(e) => setCount(Number(e.target.value))} />
        </label>
        {error && <p className="error">{error}</p>}
        <ModalActions
          onCancel={onClose}
          submitLabel="Generate"
          busyLabel="Generating…"
          busy={generating}
          disabled={!model || !seedInput.trim() || !seedOutput.trim()}
        />
      </form>
    </Modal>
  )
}

export default DatasetDetail
