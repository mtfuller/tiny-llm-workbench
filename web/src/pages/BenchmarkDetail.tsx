import { ArrowDown, ArrowUp, ArrowUpDown, Pencil, Plus, Sparkles, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  addTestCases,
  deleteTestCase,
  generateTestCases,
  getBenchmark,
  getBenchmarkResults,
  listBenchmarkRuns,
  listModels,
  startBenchmarkRun,
  updateTestCase,
  type Assertion,
  type Benchmark,
  type BenchmarkRun,
  type BenchmarkRunResult,
  type Model,
  type TestCase,
  type TestCaseResult,
} from '../api'
import { useConfirm } from '../ConfirmDialog'
import { useEventStream } from '../eventStream'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { suggestedModels } from '../suggestedModels'
import TagFilterDropdown from '../TagFilterDropdown'
import TagInput from '../TagInput'
import {
  AssertionFields,
  emptyAssertion,
  formatAssertion,
  toDraftAssertions,
  toPayloadAssertions,
  type DraftAssertion,
} from '../TestCaseEditor'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'

type Tab = 'testCases' | 'results'
type SortKey = 'modelName' | 'benchmarkVersion' | 'passRate' | 'duration' | 'startedAt'
type SortDir = 'asc' | 'desc'
type TestCaseModalState = { mode: 'add' } | { mode: 'edit'; index: number } | null

const emptyTestCase: TestCase = { id: '', prompt: '', assertions: [], tags: [] }

function formatDuration(startedAt: string, finishedAt?: string): string {
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now()
  const seconds = Math.max(0, Math.round((end - new Date(startedAt).getTime()) / 1000))
  if (seconds < 60) return `${seconds}s`
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}

function passRate(r: BenchmarkRunResult): number {
  return r.total === 0 ? 0 : r.passed / r.total
}

function sortResults(results: BenchmarkRunResult[], key: SortKey, dir: SortDir): BenchmarkRunResult[] {
  const sorted = [...results].sort((a, b) => {
    switch (key) {
      case 'modelName':
        return a.modelName.localeCompare(b.modelName)
      case 'benchmarkVersion':
        return a.benchmarkVersion - b.benchmarkVersion
      case 'duration':
        return new Date(a.finishedAt).getTime() - new Date(a.startedAt).getTime() - (new Date(b.finishedAt).getTime() - new Date(b.startedAt).getTime())
      case 'startedAt':
        return new Date(a.startedAt).getTime() - new Date(b.startedAt).getTime()
      case 'passRate':
      default:
        return passRate(a) - passRate(b)
    }
  })
  return dir === 'asc' ? sorted : sorted.reverse()
}

function BenchmarkDetail() {
  const { name = '' } = useParams<{ name: string }>()
  const { subscribe } = useEventStream()
  const confirm = useConfirm()
  const showToast = useToast()

  const [tab, setTab] = useState<Tab>('testCases')
  const [benchmark, setBenchmark] = useState<Benchmark | null>(null)
  const [models, setModels] = useState<Model[]>([])
  const [results, setResults] = useState<BenchmarkRunResult[] | null>(null)
  const [runs, setRuns] = useState<BenchmarkRun[]>([])
  const [runModalOpen, setRunModalOpen] = useState(false)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [testCaseSearch, setTestCaseSearch] = useState('')
  const [activeTags, setActiveTags] = useState<Set<string>>(new Set())
  const [resultSearch, setResultSearch] = useState('')
  const [sortKey, setSortKey] = useState<SortKey>('passRate')
  const [sortDir, setSortDir] = useState<SortDir>('desc')

  const [testCaseModal, setTestCaseModal] = useState<TestCaseModalState>(null)
  const [testCaseModalSaving, setTestCaseModalSaving] = useState(false)
  const [testCaseModalError, setTestCaseModalError] = useState<string | null>(null)
  const [deletingTestCaseIndex, setDeletingTestCaseIndex] = useState<number | null>(null)

  const [generateOpen, setGenerateOpen] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [generateError, setGenerateError] = useState<string | null>(null)

  const [selectedResult, setSelectedResult] = useState<BenchmarkRunResult | null>(null)

  const reloadBenchmark = useCallback(() => {
    getBenchmark(name)
      .then(setBenchmark)
      .catch((err: Error) => setError(err.message))
  }, [name])

  const reloadResults = useCallback(() => {
    getBenchmarkResults(name)
      .then(setResults)
      .catch((err: Error) => setError(err.message))
  }, [name])

  useEffect(() => {
    reloadBenchmark()
    reloadResults()
    listModels()
      .then(setModels)
      .catch(() => setModels([]))
    listBenchmarkRuns()
      .then((all) => setRuns(all.filter((r) => r.benchmarkName === name)))
      .catch(() => setRuns([]))
  }, [name, reloadBenchmark, reloadResults])

  useEffect(() => {
    const unsubscribeStatus = subscribe('benchmark.status', (event) => {
      const run = JSON.parse(event.data) as BenchmarkRun
      if (run.benchmarkName !== name) return
      setRuns((prev) => {
        const others = prev.filter((r) => r.id !== run.id)
        return [run, ...others]
      })
      if (run.status !== 'running') reloadResults()
    })
    return unsubscribeStatus
  }, [subscribe, name, reloadResults])

  const activeRun = useMemo(() => runs.find((r) => r.status === 'running'), [runs])

  // SSE can race a fast-finishing run (same lesson learned building
  // Training/Evaluations); poll while a run for this benchmark is active so
  // the UI always reconciles, including refreshing the persisted results
  // once it finishes.
  useEffect(() => {
    if (!activeRun) return
    const interval = setInterval(() => {
      listBenchmarkRuns()
        .then((all) => {
          const mine = all.filter((r) => r.benchmarkName === name)
          setRuns(mine)
          if (!mine.some((r) => r.status === 'running')) reloadResults()
        })
        .catch(() => {})
    }, 3000)
    return () => clearInterval(interval)
  }, [activeRun, name, reloadResults])

  const modelOptions = useMemo(() => {
    const trained = models.map((m) => m.name)
    return Array.from(new Set([...trained, ...suggestedModels]))
  }, [models])

  const testCases = benchmark?.testCases ?? []

  const allTags = useMemo(() => {
    const tags = new Set<string>()
    for (const tc of testCases) {
      for (const t of tc.tags ?? []) tags.add(t)
    }
    return Array.from(tags).sort()
  }, [testCases])

  const toggleTagFilter = (tag: string) => {
    setActiveTags((prev) => {
      const next = new Set(prev)
      if (next.has(tag)) next.delete(tag)
      else next.add(tag)
      return next
    })
  }

  const handleSaveTestCaseModal = async (tc: { prompt: string; assertions: Assertion[]; tags?: string[] }) => {
    if (!testCaseModal) return

    setTestCaseModalSaving(true)
    setTestCaseModalError(null)
    try {
      if (testCaseModal.mode === 'add') {
        await addTestCases(name, [{ id: '', ...tc }])
        showToast('Added test case')
      } else {
        await updateTestCase(name, testCaseModal.index, { id: '', ...tc })
        showToast('Saved test case')
      }
      setTestCaseModal(null)
      reloadBenchmark()
    } catch (err) {
      setTestCaseModalError((err as Error).message)
    } finally {
      setTestCaseModalSaving(false)
    }
  }

  const handleDeleteTestCase = async (index: number) => {
    if (!(await confirm('Delete this test case? This cannot be undone.'))) return

    setDeletingTestCaseIndex(index)
    setError(null)
    try {
      await deleteTestCase(name, index)
      showToast('Deleted test case')
      reloadBenchmark()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setDeletingTestCaseIndex(null)
    }
  }

  const handleGenerate = async (req: { model: string; seedPrompt: string; assertions: Assertion[]; tags?: string[]; count: number }) => {
    setGenerating(true)
    setGenerateError(null)
    try {
      await generateTestCases(name, req)
      showToast(`Generated ${req.count} test case${req.count === 1 ? '' : 's'}`)
      setGenerateOpen(false)
      reloadBenchmark()
    } catch (err) {
      setGenerateError((err as Error).message)
    } finally {
      setGenerating(false)
    }
  }

  const handleRun = async (modelNames: string[]) => {
    setStarting(true)
    setError(null)
    try {
      const run = await startBenchmarkRun(name, modelNames)
      setRuns((prev) => [run, ...prev])
      setRunModalOpen(false)
      setTab('results')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setStarting(false)
    }
  }

  const toggleSort = (key: SortKey) => {
    if (key === sortKey) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(key)
      setSortDir('desc')
    }
  }

  const sortIcon = (key: SortKey) => {
    if (key !== sortKey) return <ArrowUpDown size={13} className="sort-icon sort-icon-inactive" />
    return sortDir === 'asc' ? <ArrowUp size={13} className="sort-icon" /> : <ArrowDown size={13} className="sort-icon" />
  }

  const filteredTestCases = useMemo(() => {
    const q = testCaseSearch.trim().toLowerCase()
    return testCases
      .map((tc, index) => ({ tc, index }))
      .filter(({ tc }) => {
        if (activeTags.size > 0) {
          const tags = tc.tags ?? []
          if (![...activeTags].some((t) => tags.includes(t))) return false
        }
        if (q && !tc.prompt.toLowerCase().includes(q)) return false
        return true
      })
  }, [testCases, testCaseSearch, activeTags])

  const {
    page: testCasePage,
    setPage: setTestCasePage,
    resetPage: resetTestCasePage,
    pageCount: testCasePageCount,
    pageItems: testCasePageItems,
  } = usePagination(filteredTestCases)
  useEffect(resetTestCasePage, [testCaseSearch, activeTags, resetTestCasePage])

  const filteredResults = useMemo(() => {
    const q = resultSearch.trim().toLowerCase()
    const base = results ?? []
    const matching = q ? base.filter((r) => r.modelName.toLowerCase().includes(q)) : base
    return sortResults(matching, sortKey, sortDir)
  }, [results, resultSearch, sortKey, sortDir])

  const {
    page: resultPage,
    setPage: setResultPage,
    resetPage: resetResultPage,
    pageCount: resultPageCount,
    pageItems: resultPageItems,
  } = usePagination(filteredResults)
  useEffect(resetResultPage, [resultSearch, resetResultPage])

  return (
    <>
      <div className="page-header">
        <h2>
          <Link to="/benchmarks">Benchmarks</Link> / {name}
          {benchmark && <span className="badge version-badge">v{benchmark.version}</span>}
        </h2>
      </div>

      {error && <p className="error">{error}</p>}

      {benchmark && (
        <section className="panel">
          <h3>Benchmark info</h3>
          <dl className="info-list">
            <dt>Version</dt>
            <dd>v{benchmark.version}</dd>
            <dt>Test cases</dt>
            <dd>{benchmark.testCases.length}</dd>
            <dt>Created</dt>
            <dd>{new Date(benchmark.createdAt).toLocaleString()}</dd>
          </dl>
        </section>
      )}

      <div className="tab-bar">
        <button type="button" className={`tab-button${tab === 'testCases' ? ' tab-button-active' : ''}`} onClick={() => setTab('testCases')}>
          Test cases
        </button>
        <button type="button" className={`tab-button${tab === 'results' ? ' tab-button-active' : ''}`} onClick={() => setTab('results')}>
          Run results
        </button>
      </div>

      {tab === 'testCases' && (
        <div className="panel panel-flush">
          <div className="list-toolbar panel-toolbar">
            <input
              type="search"
              placeholder="Search test cases…"
              value={testCaseSearch}
              onChange={(e) => setTestCaseSearch(e.target.value)}
              className="list-search"
            />
            {allTags.length > 0 && (
              <TagFilterDropdown tags={allTags} active={activeTags} onToggle={toggleTagFilter} onClear={() => setActiveTags(new Set())} />
            )}
            <div className="list-toolbar-actions">
              <button
                type="button"
                className="icon-button"
                title="Add test case"
                aria-label="Add test case"
                onClick={() => setTestCaseModal({ mode: 'add' })}
              >
                <Plus size={16} />
              </button>
              <button
                type="button"
                className="icon-button"
                title="Generate test cases…"
                aria-label="Generate test cases"
                onClick={() => setGenerateOpen(true)}
              >
                <Sparkles size={16} />
              </button>
            </div>
          </div>

          {benchmark === null && (
            <div className="panel-body">
              <TableSkeleton columns={4} />
            </div>
          )}

          {benchmark !== null && testCases.length === 0 && (
            <div className="panel-body">
              <p className="hint">No test cases yet. Add one above or generate some.</p>
            </div>
          )}

          {benchmark !== null && testCases.length > 0 && filteredTestCases.length === 0 && (
            <div className="panel-body">
              <p className="hint">No test cases match your search/filter.</p>
            </div>
          )}

          {filteredTestCases.length > 0 && (
            <table className="data-table">
              <thead>
                <tr>
                  <th>Prompt</th>
                  <th>Assertions</th>
                  <th>Tags</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {testCasePageItems.map(({ tc, index }) => (
                  <tr key={tc.id}>
                    <td className="cell-truncate">{tc.prompt}</td>
                    <td>
                      {tc.assertions.map((a, i) => (
                        <div key={i}>
                          <code title={a.type === 'json_schema' ? a.value : undefined}>{formatAssertion(a)}</code>
                        </div>
                      ))}
                    </td>
                    <td>
                      {tc.tags && tc.tags.length > 0 ? (
                        <div className="tag-list">
                          {tc.tags.map((tag) => (
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
                        aria-label="Edit test case"
                        onClick={() => setTestCaseModal({ mode: 'edit', index })}
                      >
                        <Pencil size={15} />
                      </button>
                      <button
                        type="button"
                        className="icon-button"
                        aria-label="Delete test case"
                        disabled={deletingTestCaseIndex === index}
                        onClick={() => handleDeleteTestCase(index)}
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
            page={testCasePage}
            pageCount={testCasePageCount}
            onChange={setTestCasePage}
            shownCount={filteredTestCases.length}
            totalCount={testCases.length}
            itemLabel="test cases"
          />
        </div>
      )}

      {tab === 'results' && (
        <div className="panel panel-flush">
          <div className="list-toolbar panel-toolbar">
            <input
              type="search"
              placeholder="Search by model…"
              value={resultSearch}
              onChange={(e) => setResultSearch(e.target.value)}
              className="list-search"
            />
            <div className="list-toolbar-actions">
              <button
                type="button"
                className="icon-button"
                title={activeRun ? 'A run is already in progress' : 'Run benchmark'}
                aria-label="Run benchmark"
                disabled={!!activeRun}
                onClick={() => setRunModalOpen(true)}
              >
                <Plus size={16} />
              </button>
            </div>
          </div>

          {results === null && (
            <div className="panel-body">
              <TableSkeleton columns={5} />
            </div>
          )}

          {results !== null && results.length === 0 && (
            <div className="panel-body">
              <p className="hint">No runs yet. Click + above to run the benchmark against one or more models.</p>
            </div>
          )}

          {results !== null && results.length > 0 && filteredResults.length === 0 && (
            <div className="panel-body">
              <p className="hint">No results match your search.</p>
            </div>
          )}

          {filteredResults.length > 0 && (
            <table className="data-table">
              <thead>
                <tr>
                  <th>
                    <button type="button" className="sort-header" onClick={() => toggleSort('modelName')}>
                      Model {sortIcon('modelName')}
                    </button>
                  </th>
                  <th>
                    <button type="button" className="sort-header" onClick={() => toggleSort('benchmarkVersion')}>
                      Version {sortIcon('benchmarkVersion')}
                    </button>
                  </th>
                  <th>
                    <button type="button" className="sort-header" onClick={() => toggleSort('passRate')}>
                      Pass rate {sortIcon('passRate')}
                    </button>
                  </th>
                  <th>
                    <button type="button" className="sort-header" onClick={() => toggleSort('duration')}>
                      Duration {sortIcon('duration')}
                    </button>
                  </th>
                  <th>
                    <button type="button" className="sort-header" onClick={() => toggleSort('startedAt')}>
                      Started {sortIcon('startedAt')}
                    </button>
                  </th>
                </tr>
              </thead>
              <tbody>
                {resultPageItems.map((r) => (
                  <tr key={`${r.benchmarkVersion}-${r.modelName}`}>
                    <td>
                      <button type="button" className="result-cell-text" onClick={() => setSelectedResult(r)}>
                        {r.modelName}
                      </button>
                    </td>
                    <td>
                      v{r.benchmarkVersion}
                      {benchmark && r.benchmarkVersion !== benchmark.version && <span className="hint outdated-badge"> (outdated)</span>}
                    </td>
                    <td>
                      {r.passed}/{r.total} ({Math.round(passRate(r) * 100)}%)
                    </td>
                    <td>{formatDuration(r.startedAt, r.finishedAt)}</td>
                    <td>{new Date(r.startedAt).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          <Pagination
            page={resultPage}
            pageCount={resultPageCount}
            onChange={setResultPage}
            shownCount={filteredResults.length}
            totalCount={results?.length ?? 0}
            itemLabel="results"
          />
        </div>
      )}

      {testCaseModal && (
        <TestCaseModal
          title={testCaseModal.mode === 'add' ? 'Add test case' : 'Edit test case'}
          initial={testCaseModal.mode === 'edit' ? testCases[testCaseModal.index] : emptyTestCase}
          allTags={allTags}
          saving={testCaseModalSaving}
          error={testCaseModalError}
          onSave={handleSaveTestCaseModal}
          onClose={() => setTestCaseModal(null)}
        />
      )}

      {generateOpen && (
        <GenerateTestCasesModal
          modelOptions={modelOptions}
          allTags={allTags}
          generating={generating}
          error={generateError}
          onGenerate={handleGenerate}
          onClose={() => setGenerateOpen(false)}
        />
      )}

      {runModalOpen && (
        <RunBenchmarkModal models={models} starting={starting} error={error} onRun={handleRun} onClose={() => setRunModalOpen(false)} />
      )}

      {selectedResult && (
        <Modal title={`${selectedResult.modelName} — v${selectedResult.benchmarkVersion}`} onClose={() => setSelectedResult(null)} size="lg">
          {selectedResult.error && <p className="error">{selectedResult.error}</p>}
          <div className="benchmark-result-detail">
            {selectedResult.results.map((tcResult) => (
              <TestCaseResultCard key={tcResult.testCaseId} result={tcResult} />
            ))}
          </div>
        </Modal>
      )}
    </>
  )
}

interface TestCaseModalProps {
  title: string
  initial: TestCase
  allTags: string[]
  saving: boolean
  error: string | null
  onSave: (tc: { prompt: string; assertions: Assertion[]; tags?: string[] }) => void
  onClose: () => void
}

// TestCaseModal is the single prompt/assertions/tags editing surface used
// for both adding a new test case and editing an existing one — mirrors
// DatasetDetail's ExampleModal.
function TestCaseModal({ title, initial, allTags, saving, error, onSave, onClose }: TestCaseModalProps) {
  const [prompt, setPrompt] = useState(initial.prompt)
  const [assertions, setAssertions] = useState<DraftAssertion[]>(toDraftAssertions(initial.assertions))
  const [tags, setTags] = useState<string[]>(initial.tags ?? [])

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!prompt.trim()) return
    onSave({ prompt: prompt.trim(), assertions: toPayloadAssertions(assertions), tags: tags.length > 0 ? tags : undefined })
  }

  return (
    <Modal title={title} onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Prompt
          <textarea rows={3} value={prompt} onChange={(e) => setPrompt(e.target.value)} autoFocus />
        </label>
        <AssertionFields assertions={assertions} onChange={setAssertions} />
        <label>
          Tags
          <TagInput tags={tags} onChange={setTags} suggestions={allTags} />
        </label>
        {error && <p className="error">{error}</p>}
        <div className="row-actions confirm-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={saving || !prompt.trim()}>
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

interface GenerateTestCasesModalProps {
  modelOptions: string[]
  allTags: string[]
  generating: boolean
  error: string | null
  onGenerate: (req: { model: string; seedPrompt: string; assertions: Assertion[]; tags?: string[]; count: number }) => void
  onClose: () => void
}

function GenerateTestCasesModal({ modelOptions, allTags, generating, error, onGenerate, onClose }: GenerateTestCasesModalProps) {
  const [model, setModel] = useState('')
  const [seedPrompt, setSeedPrompt] = useState('')
  const [assertions, setAssertions] = useState<DraftAssertion[]>([emptyAssertion()])
  const [tags, setTags] = useState<string[]>([])
  const [count, setCount] = useState(3)

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!model || !seedPrompt.trim()) return
    onGenerate({
      model,
      seedPrompt: seedPrompt.trim(),
      assertions: toPayloadAssertions(assertions),
      tags: tags.length > 0 ? tags : undefined,
      count,
    })
  }

  return (
    <Modal title="Generate test cases" onClose={onClose}>
      <p className="hint">
        Give one example prompt and the assertions it should satisfy — a local model will generate differently
        phrased prompts that test the same thing, keeping the same assertions on every one of them.
      </p>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Model
          <input
            type="text"
            list="generate-test-case-model-options"
            placeholder="mlx-community/Qwen2.5-0.5B-Instruct-4bit"
            value={model}
            onChange={(e) => setModel(e.target.value)}
          />
          <datalist id="generate-test-case-model-options">
            {modelOptions.map((name) => (
              <option key={name} value={name} />
            ))}
          </datalist>
        </label>
        <label>
          Example prompt
          <textarea rows={2} value={seedPrompt} onChange={(e) => setSeedPrompt(e.target.value)} />
        </label>
        <AssertionFields assertions={assertions} onChange={setAssertions} />
        <label>
          Tags
          <TagInput tags={tags} onChange={setTags} suggestions={allTags} />
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

interface RunBenchmarkModalProps {
  models: Model[]
  starting: boolean
  error: string | null
  onRun: (modelNames: string[]) => void
  onClose: () => void
}

function RunBenchmarkModal({ models, starting, error, onRun, onClose }: RunBenchmarkModalProps) {
  const [selectedModels, setSelectedModels] = useState<string[]>([])

  const toggleModel = (modelName: string) => {
    setSelectedModels((prev) => (prev.includes(modelName) ? prev.filter((m) => m !== modelName) : [...prev, modelName]))
  }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (selectedModels.length === 0) return
    onRun(selectedModels)
  }

  return (
    <Modal title="Run benchmark" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        {models.length === 0 && <p className="empty-state">No models available. Train or import one on the Models page.</p>}
        {models.length > 0 && (
          <div className="agent-checklist">
            {models.map((m) => (
              <label key={m.name} className="checkbox-label">
                <input type="checkbox" checked={selectedModels.includes(m.name)} onChange={() => toggleModel(m.name)} />
                {m.name}
              </label>
            ))}
          </div>
        )}
        {error && <p className="error">{error}</p>}
        <div className="row-actions confirm-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={starting || selectedModels.length === 0}>
            {starting ? 'Starting…' : 'Run benchmark'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

interface TestCaseResultCardProps {
  result: TestCaseResult
}

function TestCaseResultCard({ result }: TestCaseResultCardProps) {
  return (
    <div className="test-case-editor">
      <div className="page-header">
        <strong>{result.prompt}</strong>
        <span className={`badge ${result.passed ? 'badge-purple' : ''}`}>{result.passed ? 'pass' : 'fail'}</span>
      </div>
      <p className="hint">Reply</p>
      <pre className="exec-output">{result.reply || result.error || '(no reply)'}</pre>
      <p className="hint">Assertions</p>
      <ul className="event-log">
        {result.assertions.map((a, i) => (
          <li key={i}>
            <span className={`badge ${a.passed ? 'badge-purple' : ''}`}>{a.passed ? 'pass' : 'fail'}</span>
            <span className="event-data">
              <code title={a.type === 'json_schema' ? a.value : undefined}>{formatAssertion(a)}</code>
              {a.error && <div className="error">{a.error}</div>}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

export default BenchmarkDetail
