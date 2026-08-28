import { Pencil, Plus, Sparkles, Tag, Trash2 } from 'lucide-react'
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
import IconButton from '../IconButton'
import Modal from '../Modal'
import ModalActions from '../ModalActions'
import ModelCombobox from '../ModelCombobox'
import Pagination from '../Pagination'
import SortableHeader from '../SortableHeader'
import TabBar from '../TabBar'
import { VersionBadge } from '../Badge'
import { formatDuration, formatMs } from '../lib/format'
import { compareByMetricKey, computeMetrics, type MetricSortKey } from '../lib/resultMetrics'
import { TableSkeleton } from '../Skeleton'
import TagCell from '../TagCell'
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
import { useTagFilter } from '../useTagFilter'

type Tab = 'testCases' | 'results'
type SortKey = 'modelName' | 'benchmarkVersion' | MetricSortKey
type SortDir = 'asc' | 'desc'
type TestCaseModalState = { mode: 'add' } | { mode: 'edit'; index: number } | null

const emptyTestCase: TestCase = { id: '', prompt: '', assertions: [], tags: [] }

function sortResults(results: BenchmarkRunResult[], key: SortKey, dir: SortDir): BenchmarkRunResult[] {
  const sorted = [...results].sort((a, b) => {
    switch (key) {
      case 'modelName':
        return a.modelName.localeCompare(b.modelName)
      case 'benchmarkVersion':
        return a.benchmarkVersion - b.benchmarkVersion
      default:
        return compareByMetricKey(a, b, key)
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

  const modelNames = useMemo(() => models.map((m) => m.name), [models])

  const testCases = benchmark?.testCases ?? []

  const { allTags, activeTags, toggleTag, clearTags, matchesTags } = useTagFilter(testCases, (tc) => tc.tags)

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

  const filteredTestCases = useMemo(() => {
    const q = testCaseSearch.trim().toLowerCase()
    return testCases
      .map((tc, index) => ({ tc, index }))
      .filter(({ tc }) => {
        if (!matchesTags(tc)) return false
        if (q && !tc.prompt.toLowerCase().includes(q)) return false
        return true
      })
  }, [testCases, testCaseSearch, matchesTags])

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
          {benchmark && <VersionBadge version={benchmark.version} />}
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

      <TabBar
        tabs={[
          { value: 'testCases', label: 'Test cases' },
          { value: 'results', label: 'Run results' },
        ]}
        value={tab}
        onChange={setTab}
      />

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
                groups={[{ key: 'tags', title: 'Tags', options: allTags, active: activeTags, onToggle: toggleTag }]}
                onClearAll={clearTags}
              />
            )}
            <div className="list-toolbar-actions">
              <IconButton icon={<Plus size={16} />} label="Add test case" onClick={() => setTestCaseModal({ mode: 'add' })} />
              <IconButton icon={<Sparkles size={16} />} label="Generate test cases…" onClick={() => setGenerateOpen(true)} />
              <IconButton
                icon={<Tag size={16} />}
                label={testCases.length === 0 ? 'Add a test case before publishing' : 'Publish version — freezes the current draft'}
                disabled={publishing || testCases.length === 0}
                onClick={handlePublish}
              />
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
                      <TagCell tags={tc.tags} />
                    </td>
                    <td className="row-actions">
                      <IconButton
                        icon={<Pencil size={15} />}
                        label="Edit test case"
                        onClick={() => setTestCaseModal({ mode: 'edit', index })}
                      />
                      <IconButton
                        icon={<Trash2 size={15} />}
                        label="Delete test case"
                        disabled={deletingTestCaseIndex === index}
                        onClick={() => handleDeleteTestCase(index)}
                      />
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
              <IconButton
                icon={<Plus size={16} />}
                label={
                  activeRun
                    ? 'A run is already in progress'
                    : !benchmark || benchmark.version === 0
                      ? 'Publish a version first (Test cases tab)'
                      : 'Run benchmark'
                }
                disabled={!!activeRun || !benchmark || benchmark.version === 0}
                onClick={() => setRunModalOpen(true)}
              />
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
                    <SortableHeader label="Model" columnKey="modelName" activeKey={sortKey} dir={sortDir} onSort={toggleSort} />
                    <SortableHeader
                      label="Version"
                      columnKey="benchmarkVersion"
                      activeKey={sortKey}
                      dir={sortDir}
                      onSort={toggleSort}
                    />
                    <SortableHeader
                      label="Pass@1"
                      columnKey="passAt1"
                      activeKey={sortKey}
                      dir={sortDir}
                      onSort={toggleSort}
                      title="pass@1 — fraction of test cases passed, k=1 sample"
                    />
                    <SortableHeader
                      label="Assertion rate"
                      columnKey="assertionRate"
                      activeKey={sortKey}
                      dir={sortDir}
                      onSort={toggleSort}
                      title="Fraction of individual assertions passed, across all test cases"
                    />
                    <SortableHeader
                      label="Errors"
                      columnKey="errorRate"
                      activeKey={sortKey}
                      dir={sortDir}
                      onSort={toggleSort}
                      title="Fraction of test cases that errored (no reply at all)"
                    />
                    <SortableHeader label="Avg latency" columnKey="avgLatency" activeKey={sortKey} dir={sortDir} onSort={toggleSort} />
                    <SortableHeader label="Started" columnKey="startedAt" activeKey={sortKey} dir={sortDir} onSort={toggleSort} />
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
          models={modelNames}
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
        <ModalActions onCancel={onClose} busy={saving} disabled={!prompt.trim()} />
      </form>
    </Modal>
  )
}

interface GenerateTestCasesModalProps {
  models: string[]
  allTags: string[]
  generating: boolean
  error: string | null
  onGenerate: (req: { model: string; seedPrompt: string; assertions: Assertion[]; tags?: string[]; count: number }) => void
  onClose: () => void
}

function GenerateTestCasesModal({ models, allTags, generating, error, onGenerate, onClose }: GenerateTestCasesModalProps) {
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
          <ModelCombobox value={model} onChange={setModel} models={models} />
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
        <ModalActions onCancel={onClose} submitLabel="Generate" busyLabel="Generating…" busy={generating} disabled={!model} />
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
        <ModalActions
          onCancel={onClose}
          submitLabel="Run benchmark"
          busyLabel="Starting…"
          busy={starting}
          disabled={selectedModels.length === 0 || version <= 0}
        />
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
