// Shared benchmark/evaluation run-result metrics. Benchmark and Evaluation
// run results are structurally the same (a list of per-test-case results
// with pass counts, assertion outcomes, an error flag, and start/finish
// timestamps), so the metric math lives here once. Evaluation results also
// carry verify-step assertions; `verifyResults` is optional, so the same
// function serves both (it's simply empty for benchmarks).

interface AssertionOutcome {
  passed: boolean
}

interface TestCaseResultLike {
  assertions: AssertionOutcome[]
  verifyResults?: { assertions: AssertionOutcome[] }[]
  error?: string
}

export interface RunResultLike {
  total: number
  passed: number
  startedAt: string
  finishedAt: string
  results: TestCaseResultLike[]
}

// ResultMetrics are standard aggregates derived entirely from a run
// result's own per-test-case data — no extra backend data, so historical
// results get them for free.
//
// passAt1 is pass@k with k=1: since each test case runs exactly once per
// target, it's mathematically identical to the plain pass rate.
// assertionRate is finer-grained — a test case with several assertions
// gets partial credit even when it doesn't pass outright. avgLatencyMs
// divides the run's own wall-clock duration by its test case count (exact,
// since test cases run strictly sequentially).
export interface ResultMetrics {
  passAt1: number
  assertionRate: number
  errorRate: number
  avgLatencyMs: number
}

export function computeMetrics(r: RunResultLike): ResultMetrics {
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

// MetricSortKey covers the sort columns shared by both results tables. Each
// page keeps its own two domain columns (model/agent name, version) and
// delegates the rest here.
export type MetricSortKey = 'passAt1' | 'assertionRate' | 'errorRate' | 'avgLatency' | 'startedAt'

export function compareByMetricKey(a: RunResultLike, b: RunResultLike, key: MetricSortKey): number {
  switch (key) {
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
}
