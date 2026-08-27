import { ArrowDown, ArrowUp, ArrowUpDown, Pencil, Plus, Sparkles, Tag, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  addTestCases,
  deleteTestCase,
  generateTestCases,
  getBenchmark,
  getBenchmarkResults,
  listBenchmarkRuns,
  listBenchmarkVersions,
  listModels,
  publishBenchmarkVersion,
  startBenchmarkRun,
  updateTestCase,
  type Assertion,
  type Benchmark,
  type BenchmarkRun,
  type BenchmarkRunResult,
  type BenchmarkVersion,
  type Model,
  type TestCase,
  type TestCaseResult,
} from '../api'
import { useConfirm } from '../ConfirmDialog'
import { useEventStream } from '../eventStream'
import FilterMenu from '../FilterMenu'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import { suggestedModels } from '../suggestedModels'
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
type SortKey = 'modelName' | 'benchmarkVersion' | 'passAt1' | 'assertionRate' | 'errorRate' | 'avgLatency' | 'startedAt'
type SortDir = 'asc' | 'desc'
type TestCaseModalState = { mode: 'add' } | { mode: 'edit'; index: number } | null

const emptyTestCase: TestCase = { id: '', prompt: '', assertions: [], tags: [] }

function formatDuration(startedAt: string, finishedAt?: string): string {
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now()
  const seconds = Math.max(0, Math.round((end - new Date(startedAt).getTime()) / 1000))
  if (seconds < 60) return `${seconds}s`
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}

function formatMs(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

// ResultMetrics are standard benchmark-style aggregates derived entirely
// from a RunResult's own per-test-case results — no extra data needed from
// the backend, so these compute the same way for historical results too.
//
// passAt1 is the well-known "pass@k" metric with k=1: since each test case
// here runs exactly once per model, pass@1 is mathematically identical to
// the plain pass rate (passed/total) — that's what pass@1 means at k=1, not
// an approximation of it. assertionRate is a finer-grained score than
// passAt1: a test case with several assertions counts partial credit here
// even when it doesn't pass outright. avgLatencyMs divides the run's own
// wall-clock duration by its test case count — exact, not approximate,
// since test cases run strictly sequentially (see internal/benchmarks).
interface ResultMetrics {
  passAt1: number
  assertionRate: number
  errorRate: number
  avgLatencyMs: number
}

function computeMetrics(r: BenchmarkRunResult): ResultMetrics {
  const total = r.total || 1
  let assertionTotal = 0
  let assertionPassed = 0
  let errorCount = 0
  for (const tc of r.results) {
    assertionTotal += tc.assertions.length
    assertionPassed += tc.assertions.filter((a) => a.passed).length
    if (tc.error) errorCount++
  }
  const durationMs = new Date(r.finishedAt).getTime() - new Date(r.startedAt).getTime()

  return {
    passAt1: r.total === 0 ? 0 : r.passed / r.total,
    assertionRate: assertionTotal === 0 ? 0 : assertionPassed / assertionTotal,
    errorRate: errorCount / total,
    avgLatencyMs: durationMs / total,
  }
}

function sortResults(results: BenchmarkRunResult[], key: SortKey, dir: SortDir): BenchmarkRunResult[] {
  const sorted = [...results].sort((a, b) => {
    switch (key) {
      case 'modelName':
        return a.modelName.localeCompare(b.modelName)
      case 'benchmarkVersion':
        return a.benchmarkVersion - b.benchmarkVersion
      case 'assertionRate':
        return computeMetrics(a).assertionRate - computeMetrics(b).assertionRate
      case 'errorRate':
        return computeMetrics(a).errorRate - computeMetrics(b).errorRate
      case 'avgLatency':
        return computeMetrics(a).avgLatencyMs - computeMetrics(b).avgLatencyMs
      case 'startedAt':
        return new Date(a.startedAt).getTime() - new Date(b.startedAt).getTime()
      case 'passAt1':
      default:
        return computeMetrics(a).passAt1 - computeMetrics(b).passAt1
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
  const [versions, setVersions] = useState<BenchmarkVersion[]>([])
  const [models, setModels] = useState<Model[]>([])
  const [results, setResults] = useState<BenchmarkRunResult[] | null>(null)
  const [runs, setRuns] = useState<BenchmarkRun[]>([])
  const [runModalOpen, setRunModalOpen] = useState(false)
  const [starting, setStarting] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [testCaseSearch, setTestCaseSearch] = useState('')
  const [activeTags, setActiveTags] = useState<Set<string>>(new Set())
  const [resultSearch, setResultSearch] = useState('')
  const [sortKey, setSortKey] = useState<SortKey>('passAt1')
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

  const reloadVersions = useCallback(() => {
    listBenchmarkVersions(name)
      .then(setVersions)
      .catch(() => setVersions([]))
  }, [name])

  const reloadResults = useCallback(() => {
    getBenchmarkResults(name)
      .then(setResults)
      .catch((err: Error) => setError(err.message))
  }, [name])

  useEffect(() => {
    reloadBenchmark()
    reloadVersions()
    reloadResults()
    listModels()
      .then(setModels)
      .catch(() => setModels([]))
    listBenchmarkRuns()
      .then((all) => setRuns(all.filter((r) => r.benchmarkName === name)))
      .catch(() => setRuns([]))
  }, [name, reloadBenchmark, reloadVersions, reloadResults])

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

  const latestVersion = versions.length > 0 ? versions[versions.length - 1] : null
  const draftDiffersFromLatest = !latestVersion || JSON.stringify(latestVersion.testCases) !== JSON.stringify(testCases)

  const handlePublish = async () => {
    setPublishing(true)
    setError(null)
    try {
      await publishBenchmarkVersion(name)
      showToast('Published a new version')
      reloadBenchmark()
      reloadVersions()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setPublishing(false)
    }
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

  const handleRun = async (version: number, modelNames: string[]) => {
    setStarting(true)
    setError(null)
    try {
      const run = await startBenchmarkRun(name, version, modelNames)
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
          {benchmark && <span className="badge version-badge">{benchmark.version === 0 ? 'unpublished' : `v${benchmark.version}`}</span>}
        </h2>
      </div>

      {error && <p className="error">{error}</p>}

      {benchmark && (
        <section className="panel">
          <h3>Benchmark info</h3>
          <dl className="info-list">
            <dt>Published version</dt>
            <dd>{benchmark.version === 0 ? 'None yet' : `v${benchmark.version}`}</dd>
            <dt>Draft test cases</dt>
            <dd>{benchmark.testCases.length}</dd>
            <dt>Created</dt>
            <dd>{new Date(benchmark.createdAt).toLocaleString()}</dd>
          </dl>
          {testCases.length > 0 && draftDiffersFromLatest && (
            <p className="hint">
              {latestVersion
                ? `Draft test cases differ from the latest published version (v${latestVersion.version}) — publish a new version before running against these changes.`
                : 'No version has been published yet — publish one from the Test cases tab to enable running this benchmark.'}
            </p>
          )}
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
              <FilterMenu
                groups={[{ key: 'tags', title: 'Tags', options: allTags, active: activeTags, onToggle: toggleTagFilter }]}
                onClearAll={() => setActiveTags(new Set())}
              />
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
              <button
                type="button"
                className="icon-button"
                title={testCases.length === 0 ? 'Add a test case before publishing' : 'Publish version — freezes the current draft'}
                aria-label="Publish version"
                disabled={publishing || testCases.length === 0}
                onClick={handlePublish}
              >
                <Tag size={16} />
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
                title={
                  activeRun
                    ? 'A run is already in progress'
                    : !benchmark || benchmark.version === 0
                      ? 'Publish a version first (Test cases tab)'
                      : 'Run benchmark'
                }
                aria-label="Run benchmark"
                disabled={!!activeRun || !benchmark || benchmark.version === 0}
                onClick={() => setRunModalOpen(true)}
              >
                <Plus size={16} />
              </button>
            </div>
          </div>

          {results === null && (
            <div className="panel-body">
              <TableSkeleton columns={7} />
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
            <div className="table-scroll">
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
                      <button type="button" className="sort-header" onClick={() => toggleSort('passAt1')} title="pass@1 — fraction of test cases passed, k=1 sample">
                        Pass@1 {sortIcon('passAt1')}
                      </button>
                    </th>
                    <th>
                      <button
                        type="button"
                        className="sort-header"
                        onClick={() => toggleSort('assertionRate')}
                        title="Fraction of individual assertions passed, across all test cases"
                      >
                        Assertion rate {sortIcon('assertionRate')}
                      </button>
                    </th>
                    <th>
                      <button type="button" className="sort-header" onClick={() => toggleSort('errorRate')} title="Fraction of test cases that errored (no reply at all)">
                        Errors {sortIcon('errorRate')}
                      </button>
                    </th>
                    <th>
                      <button type="button" className="sort-header" onClick={() => toggleSort('avgLatency')}>
                        Avg latency {sortIcon('avgLatency')}
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
                  {resultPageItems.map((r) => {
                    const metrics = computeMetrics(r)
                    return (
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
                          {r.passed}/{r.total} ({Math.round(metrics.passAt1 * 100)}%)
                        </td>
                        <td>{Math.round(metrics.assertionRate * 100)}%</td>
                        <td>{Math.round(metrics.errorRate * 100)}%</td>
                        <td>{formatMs(metrics.avgLatencyMs)}</td>
                        <td>{new Date(r.startedAt).toLocaleString()}</td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
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

      {runModalOpen && benchmark && (
        <RunBenchmarkModal
          models={models}
          versions={versions}
          defaultVersion={benchmark.version}
          starting={starting}
          error={error}
          onRun={handleRun}
          onClose={() => setRunModalOpen(false)}
        />
      )}

      {selectedResult && (
        <Modal title={`${selectedResult.modelName} — v${selectedResult.benchmarkVersion}`} onClose={() => setSelectedResult(null)} size="lg">
          {selectedResult.error && <p className="error">{selectedResult.error}</p>}
          <dl className="info-list">
            <dt>Pass@1</dt>
            <dd>
              {selectedResult.passed}/{selectedResult.total} ({Math.round(computeMetrics(selectedResult).passAt1 * 100)}%)
            </dd>
            <dt>Assertion rate</dt>
            <dd>{Math.round(computeMetrics(selectedResult).assertionRate * 100)}%</dd>
            <dt>Errors</dt>
            <dd>{Math.round(computeMetrics(selectedResult).errorRate * 100)}%</dd>
            <dt>Avg latency</dt>
            <dd>{formatMs(computeMetrics(selectedResult).avgLatencyMs)}</dd>
            <dt>Total duration</dt>
            <dd>{formatDuration(selectedResult.startedAt, selectedResult.finishedAt)}</dd>
          </dl>
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
  versions: BenchmarkVersion[]
  defaultVersion: number
  starting: boolean
  error: string | null
  onRun: (version: number, modelNames: string[]) => void
  onClose: () => void
}

function RunBenchmarkModal({ models, versions, defaultVersion, starting, error, onRun, onClose }: RunBenchmarkModalProps) {
  const [version, setVersion] = useState(defaultVersion)
  const [selectedModels, setSelectedModels] = useState<string[]>([])

  const toggleModel = (modelName: string) => {
    setSelectedModels((prev) => (prev.includes(modelName) ? prev.filter((m) => m !== modelName) : [...prev, modelName]))
  }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (selectedModels.length === 0 || version <= 0) return
    onRun(version, selectedModels)
  }

  return (
    <Modal title="Run benchmark" onClose={onClose}>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Version
          <select value={version} onChange={(e) => setVersion(Number(e.target.value))}>
            {[...versions].reverse().map((v) => (
              <option key={v.version} value={v.version}>
                v{v.version} — published {new Date(v.publishedAt).toLocaleDateString()} ({v.testCases.length} test cases)
              </option>
            ))}
          </select>
        </label>
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
          <button type="submit" disabled={starting || selectedModels.length === 0 || version <= 0}>
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
