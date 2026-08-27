import { ArrowDown, ArrowUp, ArrowUpDown, Pencil, Plus, Sparkles, Tag, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  addEvaluationTestCases,
  deleteEvaluationTestCase,
  generateEvaluationTestCases,
  getEvaluation,
  getEvaluationResults,
  listAgents,
  listEnvironments,
  listEvaluationRuns,
  listEvaluationVersions,
  publishEvaluationVersion,
  startEvaluationRun,
  updateEvaluationConfig,
  updateEvaluationTestCase,
  type Agent,
  type Assertion,
  type Environment,
  type Evaluation,
  type EvaluationRun,
  type EvaluationRunResult,
  type EvaluationVersion,
  type TestCase,
  type TestCaseResult,
  type VerifyStep,
} from '../api'
import { useConfirm } from '../ConfirmDialog'
import { useEventStream } from '../eventStream'
import FilterMenu from '../FilterMenu'
import Modal from '../Modal'
import Pagination from '../Pagination'
import { TableSkeleton } from '../Skeleton'
import TagInput from '../TagInput'
import {
  AssertionFields,
  emptyAssertion,
  formatAssertion,
  toDraftAssertions,
  toDraftVerifySteps,
  toPayloadAssertions,
  toPayloadVerifySteps,
  VerifyStepFields,
  type DraftAssertion,
  type DraftVerifyStep,
} from '../TestCaseEditor'
import { useToast } from '../Toast'
import { usePagination } from '../usePagination'

type Tab = 'testCases' | 'results'
type SortKey = 'agentName' | 'evaluationVersion' | 'passAt1' | 'assertionRate' | 'errorRate' | 'avgLatency' | 'startedAt'
type SortDir = 'asc' | 'desc'
type TestCaseModalState = { mode: 'add' } | { mode: 'edit'; index: number } | null

const emptyTestCase: TestCase = { id: '', prompt: '', setup: [], assertions: [], verifyCommands: [], tags: [] }

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

// ResultMetrics mirrors Benchmarks' computeMetrics exactly (see
// BenchmarkDetail.tsx for the full reasoning) — passAt1 is the plain pass
// rate (each test case runs exactly once per agent), assertionRate counts
// every individual assertion (both reply and verify-step assertions) for
// finer-grained partial credit, and avgLatencyMs divides the run's own
// wall-clock duration by its test case count.
interface ResultMetrics {
  passAt1: number
  assertionRate: number
  errorRate: number
  avgLatencyMs: number
}

function computeMetrics(r: EvaluationRunResult): ResultMetrics {
  const total = r.total || 1
  let assertionTotal = 0
  let assertionPassed = 0
  let errorCount = 0
  for (const tc of r.results) {
    assertionTotal += tc.assertions.length
    assertionPassed += tc.assertions.filter((a) => a.passed).length
    for (const vs of tc.verifyResults ?? []) {
      assertionTotal += vs.assertions.length
      assertionPassed += vs.assertions.filter((a) => a.passed).length
    }
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

function sortResults(results: EvaluationRunResult[], key: SortKey, dir: SortDir): EvaluationRunResult[] {
  const sorted = [...results].sort((a, b) => {
    switch (key) {
      case 'agentName':
        return a.agentName.localeCompare(b.agentName)
      case 'evaluationVersion':
        return a.evaluationVersion - b.evaluationVersion
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

function EvaluationDetail() {
  const { name = '' } = useParams<{ name: string }>()
  const { subscribe } = useEventStream()
  const confirm = useConfirm()
  const showToast = useToast()

  const [tab, setTab] = useState<Tab>('testCases')
  const [evaluation, setEvaluation] = useState<Evaluation | null>(null)
  const [versions, setVersions] = useState<EvaluationVersion[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [environments, setEnvironments] = useState<Environment[]>([])
  const [results, setResults] = useState<EvaluationRunResult[] | null>(null)
  const [runs, setRuns] = useState<EvaluationRun[]>([])
  const [runModalOpen, setRunModalOpen] = useState(false)
  const [starting, setStarting] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [envDraft, setEnvDraft] = useState('')
  const [savingEnv, setSavingEnv] = useState(false)

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

  const [selectedResult, setSelectedResult] = useState<EvaluationRunResult | null>(null)
  const [selectedTestCaseResult, setSelectedTestCaseResult] = useState<TestCaseResult | null>(null)

  const reloadEvaluation = useCallback(() => {
    getEvaluation(name)
      .then((e) => {
        setEvaluation(e)
        setEnvDraft(e.environment ?? '')
      })
      .catch((err: Error) => setError(err.message))
  }, [name])

  const reloadVersions = useCallback(() => {
    listEvaluationVersions(name)
      .then(setVersions)
      .catch(() => setVersions([]))
  }, [name])

  const reloadResults = useCallback(() => {
    getEvaluationResults(name)
      .then(setResults)
      .catch((err: Error) => setError(err.message))
  }, [name])

  useEffect(() => {
    reloadEvaluation()
    reloadVersions()
    reloadResults()
    listAgents()
      .then(setAgents)
      .catch(() => setAgents([]))
    listEnvironments()
      .then(setEnvironments)
      .catch(() => setEnvironments([]))
    listEvaluationRuns()
      .then((all) => setRuns(all.filter((r) => r.evaluationName === name)))
      .catch(() => setRuns([]))
  }, [name, reloadEvaluation, reloadVersions, reloadResults])

  useEffect(() => {
    const unsubscribeStatus = subscribe('evaluation.status', (event) => {
      const run = JSON.parse(event.data) as EvaluationRun
      if (run.evaluationName !== name) return
      setRuns((prev) => {
        const others = prev.filter((r) => r.id !== run.id)
        return [run, ...others]
      })
      if (run.status !== 'running') reloadResults()
    })
    return unsubscribeStatus
  }, [subscribe, name, reloadResults])

  const activeRun = useMemo(() => runs.find((r) => r.status === 'running'), [runs])

  // SSE can race a fast-finishing run; poll while a run for this evaluation
  // is active so the UI always reconciles, including refreshing the
  // persisted results once it finishes.
  useEffect(() => {
    if (!activeRun) return
    const interval = setInterval(() => {
      listEvaluationRuns()
        .then((all) => {
          const mine = all.filter((r) => r.evaluationName === name)
          setRuns(mine)
          if (!mine.some((r) => r.status === 'running')) reloadResults()
        })
        .catch(() => {})
    }, 3000)
    return () => clearInterval(interval)
  }, [activeRun, name, reloadResults])

  const testCases = evaluation?.testCases ?? []

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
      await publishEvaluationVersion(name)
      showToast('Published a new version')
      reloadEvaluation()
      reloadVersions()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setPublishing(false)
    }
  }

  const handleSaveEnvironment = async () => {
    setSavingEnv(true)
    setError(null)
    try {
      await updateEvaluationConfig(name, envDraft)
      showToast('Saved configuration')
      reloadEvaluation()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSavingEnv(false)
    }
  }

  const handleSaveTestCaseModal = async (tc: {
    prompt: string
    setup?: string[]
    assertions: Assertion[]
    verifyCommands?: VerifyStep[]
    tags?: string[]
  }) => {
    if (!testCaseModal) return

    setTestCaseModalSaving(true)
    setTestCaseModalError(null)
    try {
      if (testCaseModal.mode === 'add') {
        await addEvaluationTestCases(name, [{ id: '', ...tc }])
        showToast('Added test case')
      } else {
        await updateEvaluationTestCase(name, testCaseModal.index, { id: '', ...tc })
        showToast('Saved test case')
      }
      setTestCaseModal(null)
      reloadEvaluation()
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
      await deleteEvaluationTestCase(name, index)
      showToast('Deleted test case')
      reloadEvaluation()
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
      await generateEvaluationTestCases(name, req)
      showToast(`Generated ${req.count} test case${req.count === 1 ? '' : 's'}`)
      setGenerateOpen(false)
      reloadEvaluation()
    } catch (err) {
      setGenerateError((err as Error).message)
    } finally {
      setGenerating(false)
    }
  }

  const handleRun = async (version: number, agentNames: string[]) => {
    setStarting(true)
    setError(null)
    try {
      const run = await startEvaluationRun(name, version, agentNames)
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
    const matching = q ? base.filter((r) => r.agentName.toLowerCase().includes(q)) : base
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
          <Link to="/evaluations">Evaluations</Link> / {name}
          {evaluation && <span className="badge version-badge">{evaluation.version === 0 ? 'unpublished' : `v${evaluation.version}`}</span>}
        </h2>
      </div>

      {error && <p className="error">{error}</p>}

      {evaluation && (
        <section className="panel">
          <h3>Evaluation info</h3>
          <dl className="info-list">
            <dt>Published version</dt>
            <dd>{evaluation.version === 0 ? 'None yet' : `v${evaluation.version}`}</dd>
            <dt>Draft test cases</dt>
            <dd>{evaluation.testCases.length}</dd>
            <dt>Created</dt>
            <dd>{new Date(evaluation.createdAt).toLocaleString()}</dd>
          </dl>
          <div className="stacked-form">
            <label>
              Environment (optional)
              <select value={envDraft} onChange={(e) => setEnvDraft(e.target.value)}>
                <option value="">None</option>
                {environments.map((env) => (
                  <option key={env.name} value={env.name}>
                    {env.name}
                  </option>
                ))}
              </select>
              <span className="field-hint">
                For setup/verify commands to mean anything, the agents you run against here should
                themselves be bound to this same environment — setup/verify act on the exact instance an
                agent's own Tool nodes use during its turn.
              </span>
            </label>
            <div className="row-actions confirm-actions">
              <button type="button" disabled={savingEnv || envDraft === (evaluation.environment ?? '')} onClick={handleSaveEnvironment}>
                {savingEnv ? 'Saving…' : 'Save configuration'}
              </button>
            </div>
          </div>
          {testCases.length > 0 && draftDiffersFromLatest && (
            <p className="hint">
              {latestVersion
                ? `Draft test cases differ from the latest published version (v${latestVersion.version}) — publish a new version before running against these changes.`
                : 'No version has been published yet — publish one from the Test cases tab to enable running this evaluation.'}
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

          {evaluation === null && (
            <div className="panel-body">
              <TableSkeleton columns={4} />
            </div>
          )}

          {evaluation !== null && testCases.length === 0 && (
            <div className="panel-body">
              <p className="hint">No test cases yet. Add one above or generate some.</p>
            </div>
          )}

          {evaluation !== null && testCases.length > 0 && filteredTestCases.length === 0 && (
            <div className="panel-body">
              <p className="hint">No test cases match your search/filter.</p>
            </div>
          )}

          {filteredTestCases.length > 0 && (
            <div className="table-scroll">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Prompt</th>
                    <th>Setup</th>
                    <th>Assertions</th>
                    <th>Verify</th>
                    <th>Tags</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {testCasePageItems.map(({ tc, index }) => (
                    <tr key={tc.id}>
                      <td className="cell-truncate">{tc.prompt}</td>
                      <td>{tc.setup && tc.setup.length > 0 ? `${tc.setup.length} command${tc.setup.length === 1 ? '' : 's'}` : '—'}</td>
                      <td>
                        {tc.assertions.map((a, i) => (
                          <div key={i}>
                            <code title={a.type === 'json_schema' ? a.value : undefined}>{formatAssertion(a)}</code>
                          </div>
                        ))}
                      </td>
                      <td>
                        {tc.verifyCommands && tc.verifyCommands.length > 0
                          ? `${tc.verifyCommands.length} command${tc.verifyCommands.length === 1 ? '' : 's'}`
                          : '—'}
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
            </div>
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
              placeholder="Search by agent…"
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
                    : !evaluation || evaluation.version === 0
                      ? 'Publish a version first (Test cases tab)'
                      : 'Run evaluation'
                }
                aria-label="Run evaluation"
                disabled={!!activeRun || !evaluation || evaluation.version === 0}
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
              <p className="hint">No runs yet. Click + above to run the evaluation against one or more agents.</p>
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
                      <button type="button" className="sort-header" onClick={() => toggleSort('agentName')}>
                        Agent {sortIcon('agentName')}
                      </button>
                    </th>
                    <th>
                      <button type="button" className="sort-header" onClick={() => toggleSort('evaluationVersion')}>
                        Version {sortIcon('evaluationVersion')}
                      </button>
                    </th>
                    <th>
                      <button
                        type="button"
                        className="sort-header"
                        onClick={() => toggleSort('passAt1')}
                        title="pass@1 — fraction of test cases passed, k=1 sample"
                      >
                        Pass@1 {sortIcon('passAt1')}
                      </button>
                    </th>
                    <th>
                      <button
                        type="button"
                        className="sort-header"
                        onClick={() => toggleSort('assertionRate')}
                        title="Fraction of individual assertions passed, across replies and verify steps"
                      >
                        Assertion rate {sortIcon('assertionRate')}
                      </button>
                    </th>
                    <th>
                      <button type="button" className="sort-header" onClick={() => toggleSort('errorRate')} title="Fraction of test cases that errored">
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
                      <tr key={`${r.evaluationVersion}-${r.agentName}`}>
                        <td>
                          <button type="button" className="result-cell-text" onClick={() => setSelectedResult(r)}>
                            {r.agentName}
                          </button>
                        </td>
                        <td>
                          v{r.evaluationVersion}
                          {evaluation && r.evaluationVersion !== evaluation.version && <span className="hint outdated-badge"> (outdated)</span>}
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
          generating={generating}
          error={generateError}
          onGenerate={handleGenerate}
          onClose={() => setGenerateOpen(false)}
        />
      )}

      {runModalOpen && evaluation && (
        <RunEvaluationModal
          agents={agents}
          versions={versions}
          defaultVersion={evaluation.version}
          starting={starting}
          error={error}
          onRun={handleRun}
          onClose={() => setRunModalOpen(false)}
        />
      )}

      {selectedResult && (
        <Modal title={`${selectedResult.agentName} — v${selectedResult.evaluationVersion}`} onClose={() => setSelectedResult(null)} size="lg">
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
              <TestCaseResultCard key={tcResult.testCaseId} result={tcResult} onSelect={() => setSelectedTestCaseResult(tcResult)} />
            ))}
          </div>
        </Modal>
      )}

      {selectedTestCaseResult && (
        <Modal
          title={selectedTestCaseResult.passed ? 'Test case — pass' : 'Test case — fail'}
          onClose={() => setSelectedTestCaseResult(null)}
        >
          <p className="hint">Prompt</p>
          <p>{selectedTestCaseResult.prompt}</p>

          <p className="hint">Reply</p>
          <pre className="exec-output">{selectedTestCaseResult.reply || selectedTestCaseResult.error || '(no reply)'}</pre>

          <p className="hint">Assertions</p>
          <ul className="event-log">
            {selectedTestCaseResult.assertions.map((a, i) => (
              <li key={i}>
                <span className={`badge ${a.passed ? 'badge-purple' : ''}`}>{a.passed ? 'pass' : 'fail'}</span>
                <span className="event-data">
                  <code title={a.type === 'json_schema' ? a.value : undefined}>{formatAssertion(a)}</code>
                  {a.error && <div className="error">{a.error}</div>}
                </span>
              </li>
            ))}
          </ul>

          {selectedTestCaseResult.verifyResults && selectedTestCaseResult.verifyResults.length > 0 && (
            <>
              <p className="hint">Verify steps</p>
              {selectedTestCaseResult.verifyResults.map((vs, i) => (
                <div key={i} className="test-case-editor">
                  <div className="page-header">
                    <code>{vs.command}</code>
                    <span className={`badge ${vs.passed ? 'badge-purple' : ''}`}>{vs.passed ? 'pass' : 'fail'}</span>
                  </div>
                  <pre className="exec-output">{vs.output || vs.error || '(no output)'}</pre>
                  <ul className="event-log">
                    {vs.assertions.map((a, ai) => (
                      <li key={ai}>
                        <span className={`badge ${a.passed ? 'badge-purple' : ''}`}>{a.passed ? 'pass' : 'fail'}</span>
                        <span className="event-data">
                          <code title={a.type === 'json_schema' ? a.value : undefined}>{formatAssertion(a)}</code>
                        </span>
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </>
          )}
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
  onSave: (tc: { prompt: string; setup?: string[]; assertions: Assertion[]; verifyCommands?: VerifyStep[]; tags?: string[] }) => void
  onClose: () => void
}

// TestCaseModal is the single prompt/setup/assertions/verify/tags editing
// surface used for both adding a new test case and editing an existing
// one — mirrors BenchmarkDetail's TestCaseModal, plus the two
// Evaluations-only fields (Setup, Verify steps) this feature adds.
function TestCaseModal({ title, initial, allTags, saving, error, onSave, onClose }: TestCaseModalProps) {
  const [prompt, setPrompt] = useState(initial.prompt)
  const [setupText, setSetupText] = useState((initial.setup ?? []).join('\n'))
  const [assertions, setAssertions] = useState<DraftAssertion[]>(toDraftAssertions(initial.assertions))
  const [verifySteps, setVerifySteps] = useState<DraftVerifyStep[]>(toDraftVerifySteps(initial.verifyCommands))
  const [tags, setTags] = useState<string[]>(initial.tags ?? [])

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!prompt.trim()) return
    const setup = setupText
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
    onSave({
      prompt: prompt.trim(),
      setup: setup.length > 0 ? setup : undefined,
      assertions: toPayloadAssertions(assertions),
      verifyCommands: (() => {
        const steps = toPayloadVerifySteps(verifySteps)
        return steps.length > 0 ? steps : undefined
      })(),
      tags: tags.length > 0 ? tags : undefined,
    })
  }

  return (
    <Modal title={title} onClose={onClose} size="lg">
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Prompt
          <textarea rows={3} value={prompt} onChange={(e) => setPrompt(e.target.value)} autoFocus />
        </label>

        <label>
          Setup (optional)
          <textarea
            rows={3}
            placeholder={'One shell command per line, run in order before the agent\'s turn\ne.g. mkdir -p /repo\ngit init /repo'}
            value={setupText}
            onChange={(e) => setSetupText(e.target.value)}
          />
          <span className="field-hint">
            Runs in the evaluation's Environment before the agent's turn, to prepare a realistic starting
            scenario. Requires an Environment configured above.
          </span>
        </label>

        <div className="inspector-section-label">Assertions (checked against the agent's reply)</div>
        <AssertionFields assertions={assertions} onChange={setAssertions} />

        <div className="inspector-section-label">Verify steps (optional — checked against the environment afterward)</div>
        <VerifyStepFields steps={verifySteps} onChange={setVerifySteps} />

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
  generating: boolean
  error: string | null
  onGenerate: (req: { model: string; seedPrompt: string; assertions: Assertion[]; tags?: string[]; count: number }) => void
  onClose: () => void
}

function GenerateTestCasesModal({ generating, error, onGenerate, onClose }: GenerateTestCasesModalProps) {
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
        Give one example prompt and the assertions it should satisfy — a local model will generate
        differently phrased prompts that test the same thing, keeping the same assertions (and any
        setup/verify commands are left for you to add per generated test case, since a model can't be
        trusted to write correct shell commands).
      </p>
      <form className="stacked-form" onSubmit={handleSubmit}>
        <label>
          Model
          <input
            type="text"
            placeholder="mlx-community/Qwen2.5-0.5B-Instruct-4bit"
            value={model}
            onChange={(e) => setModel(e.target.value)}
          />
        </label>
        <label>
          Example prompt
          <textarea rows={2} value={seedPrompt} onChange={(e) => setSeedPrompt(e.target.value)} />
        </label>
        <AssertionFields assertions={assertions} onChange={setAssertions} />
        <label>
          Tags
          <TagInput tags={tags} onChange={setTags} suggestions={[]} />
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

interface RunEvaluationModalProps {
  agents: Agent[]
  versions: EvaluationVersion[]
  defaultVersion: number
  starting: boolean
  error: string | null
  onRun: (version: number, agentNames: string[]) => void
  onClose: () => void
}

function RunEvaluationModal({ agents, versions, defaultVersion, starting, error, onRun, onClose }: RunEvaluationModalProps) {
  const [version, setVersion] = useState(defaultVersion)
  const [selectedAgents, setSelectedAgents] = useState<string[]>([])

  const toggleAgent = (agentName: string) => {
    setSelectedAgents((prev) => (prev.includes(agentName) ? prev.filter((a) => a !== agentName) : [...prev, agentName]))
  }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (selectedAgents.length === 0 || version <= 0) return
    onRun(version, selectedAgents)
  }

  return (
    <Modal title="Run evaluation" onClose={onClose}>
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
        {agents.length === 0 && <p className="empty-state">No agents available. Create one on the Agents page.</p>}
        {agents.length > 0 && (
          <div className="agent-checklist">
            {agents.map((a) => (
              <label key={a.name} className="checkbox-label">
                <input type="checkbox" checked={selectedAgents.includes(a.name)} onChange={() => toggleAgent(a.name)} />
                {a.name}
              </label>
            ))}
          </div>
        )}
        {error && <p className="error">{error}</p>}
        <div className="row-actions confirm-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={starting || selectedAgents.length === 0 || version <= 0}>
            {starting ? 'Starting…' : 'Run evaluation'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

interface TestCaseResultCardProps {
  result: TestCaseResult
  onSelect: () => void
}

function TestCaseResultCard({ result, onSelect }: TestCaseResultCardProps) {
  return (
    <button type="button" className="test-case-editor result-cell" onClick={onSelect}>
      <div className="page-header">
        <strong>{result.prompt}</strong>
        <span className={`badge ${result.passed ? 'badge-purple' : ''}`}>{result.passed ? 'pass' : 'fail'}</span>
      </div>
      <p className="hint">{result.error || result.reply || '(no reply)'}</p>
    </button>
  )
}

export default EvaluationDetail
