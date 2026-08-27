import type { Dispatch, SetStateAction } from 'react'
import type { Assertion, AssertionType, TestCase } from './api'

export interface DraftAssertion {
  type: AssertionType
  value: string
  // threshold is only used by "similarity" (the minimum required similarity
  // ratio, in (0, 1]).
  threshold?: number
}

export interface DraftTestCase {
  prompt: string
  assertions: DraftAssertion[]
}

const DEFAULT_SIMILARITY_THRESHOLD = 0.85

export function emptyAssertion(): DraftAssertion {
  return { type: 'contains', value: '' }
}

export function emptyTestCase(): DraftTestCase {
  return { prompt: '', assertions: [emptyAssertion()] }
}

// toDraftAssertions seeds the assertion editor from a saved test case's
// assertions.
export function toDraftAssertions(assertions: Assertion[]): DraftAssertion[] {
  if (assertions.length === 0) return [emptyAssertion()]
  return assertions.map((a) => ({ type: a.type, value: a.value, threshold: a.threshold }))
}

// toDraftTestCases seeds the editor from a saved evaluation/benchmark's test
// cases.
export function toDraftTestCases(testCases: TestCase[]): DraftTestCase[] {
  if (testCases.length === 0) return [emptyTestCase()]
  return testCases.map((tc) => ({ prompt: tc.prompt, assertions: toDraftAssertions(tc.assertions) }))
}

function toPayloadAssertion(a: DraftAssertion): Assertion {
  const assertion: Assertion = { type: a.type, value: a.value.trim() }
  if (a.type === 'similarity') {
    assertion.threshold = a.threshold ?? DEFAULT_SIMILARITY_THRESHOLD
  }
  return assertion
}

// toPayloadAssertions drops blank assertions, ready to attach to a test
// case.
export function toPayloadAssertions(drafts: DraftAssertion[]): Assertion[] {
  return drafts.filter((a) => a.value.trim()).map(toPayloadAssertion)
}

// toPayloadTestCases drops blank test cases/assertions and assigns stable
// ids, ready to send to saveEvaluation/saveBenchmark.
export function toPayloadTestCases(drafts: DraftTestCase[]): TestCase[] {
  return drafts
    .filter((tc) => tc.prompt.trim())
    .map((tc, i) => ({
      id: `tc-${i}`,
      prompt: tc.prompt.trim(),
      assertions: toPayloadAssertions(tc.assertions),
    }))
}

// formatAssertion renders a one-line, human-readable summary of an
// assertion for read-only display (results tables, detail modals).
export function formatAssertion(a: Assertion): string {
  switch (a.type) {
    case 'contains':
      return `contains "${a.value}"`
    case 'not_contains':
      return `not contains "${a.value}"`
    case 'regex':
      return `matches /${a.value}/`
    case 'json_schema':
      return 'matches JSON schema'
    case 'similarity':
      return `similar to "${a.value}" (≥ ${Math.round((a.threshold ?? DEFAULT_SIMILARITY_THRESHOLD) * 100)}%)`
    default:
      return `${a.type} "${a.value}"`
  }
}

interface AssertionFieldsProps {
  assertions: DraftAssertion[]
  onChange: (assertions: DraftAssertion[]) => void
}

// AssertionFields renders the repeated assertion rows (type + value +
// similarity threshold) for a single test case's assertions — shared
// between TestCaseFields (the multi-test-case bulk editor used by
// Evaluations) and any single-test-case editor (Benchmarks' add/edit/
// generate modals).
export function AssertionFields({ assertions, onChange }: AssertionFieldsProps) {
  const updateAssertion = (aIndex: number, patch: Partial<DraftAssertion>) => {
    onChange(assertions.map((a, ai) => (ai === aIndex ? { ...a, ...patch } : a)))
  }

  // changeAssertionType swaps an assertion's type, seeding a sensible
  // default threshold the moment it becomes "similarity" so the field is
  // never silently blank/invalid.
  const changeAssertionType = (aIndex: number, type: AssertionType) => {
    const patch: Partial<DraftAssertion> = { type }
    if (type === 'similarity') {
      patch.threshold = assertions[aIndex].threshold ?? DEFAULT_SIMILARITY_THRESHOLD
    }
    updateAssertion(aIndex, patch)
  }

  const addAssertion = () => onChange([...assertions, emptyAssertion()])
  const removeAssertion = (aIndex: number) => onChange(assertions.filter((_, ai) => ai !== aIndex))

  return (
    <>
      {assertions.map((a, aIndex) => (
        <div className="assertion-row" key={aIndex}>
          <select value={a.type} onChange={(e) => changeAssertionType(aIndex, e.target.value as AssertionType)}>
            <option value="contains">contains</option>
            <option value="not_contains">not contains</option>
            <option value="regex">matches regex</option>
            <option value="json_schema">matches JSON schema</option>
            <option value="similarity">similar to (close diff)</option>
          </select>

          {a.type === 'json_schema' ? (
            <textarea
              className="assertion-json-schema"
              placeholder='{"type": "object", "required": ["name"]}'
              rows={3}
              value={a.value}
              onChange={(e) => updateAssertion(aIndex, { value: e.target.value })}
            />
          ) : (
            <input
              type="text"
              placeholder={a.type === 'similarity' ? 'reference text' : 'value'}
              value={a.value}
              onChange={(e) => updateAssertion(aIndex, { value: e.target.value })}
            />
          )}

          {a.type === 'similarity' && (
            <input
              type="number"
              className="assertion-threshold"
              title="Minimum similarity (0-1)"
              min={0.01}
              max={1}
              step={0.01}
              value={a.threshold ?? DEFAULT_SIMILARITY_THRESHOLD}
              onChange={(e) => updateAssertion(aIndex, { threshold: Number(e.target.value) })}
            />
          )}

          {assertions.length > 1 && (
            <button type="button" className="danger-button" onClick={() => removeAssertion(aIndex)}>
              ×
            </button>
          )}
        </div>
      ))}
      <button type="button" className="button-secondary" onClick={addAssertion}>
        + Assertion
      </button>
    </>
  )
}

interface TestCaseFieldsProps {
  testCases: DraftTestCase[]
  onChange: Dispatch<SetStateAction<DraftTestCase[]>>
}

// TestCaseFields renders the repeated "test case card" editing UI (prompt +
// assertion rows) for editing several test cases at once — used by
// Evaluations' create/edit flow.
export function TestCaseFields({ testCases, onChange }: TestCaseFieldsProps) {
  const updateTestCase = (i: number, patch: Partial<DraftTestCase>) => {
    onChange((prev) => prev.map((tc, idx) => (idx === i ? { ...tc, ...patch } : tc)))
  }

  const addTestCase = () => onChange((prev) => [...prev, emptyTestCase()])
  const removeTestCase = (i: number) => onChange((prev) => prev.filter((_, idx) => idx !== i))

  return (
    <>
      {testCases.map((tc, tcIndex) => (
        <div key={tcIndex} className="test-case-editor">
          <div className="page-header">
            <strong>Test case {tcIndex + 1}</strong>
            {testCases.length > 1 && (
              <button type="button" className="danger-button" onClick={() => removeTestCase(tcIndex)}>
                Remove
              </button>
            )}
          </div>
          <label>
            Prompt
            <input
              type="text"
              placeholder="say hello"
              value={tc.prompt}
              onChange={(e) => updateTestCase(tcIndex, { prompt: e.target.value })}
            />
          </label>
          <AssertionFields assertions={tc.assertions} onChange={(assertions) => updateTestCase(tcIndex, { assertions })} />
        </div>
      ))}
      <button type="button" className="button-secondary" onClick={addTestCase}>
        + Test case
      </button>
    </>
  )
}
