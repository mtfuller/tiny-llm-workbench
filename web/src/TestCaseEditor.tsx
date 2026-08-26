import type { Dispatch, SetStateAction } from 'react'
import type { AssertionType, TestCase } from './api'

export interface DraftAssertion {
  type: AssertionType
  value: string
}

export interface DraftTestCase {
  prompt: string
  assertions: DraftAssertion[]
}

export function emptyTestCase(): DraftTestCase {
  return { prompt: '', assertions: [{ type: 'contains', value: '' }] }
}

// toDraftTestCases seeds the editor from a saved evaluation's test cases.
export function toDraftTestCases(testCases: TestCase[]): DraftTestCase[] {
  if (testCases.length === 0) return [emptyTestCase()]
  return testCases.map((tc) => ({
    prompt: tc.prompt,
    assertions:
      tc.assertions.length > 0
        ? tc.assertions.map((a) => ({ type: a.type, value: a.value }))
        : [{ type: 'contains', value: '' }],
  }))
}

// toPayloadTestCases drops blank test cases/assertions and assigns stable
// ids, ready to send to saveEvaluation.
export function toPayloadTestCases(drafts: DraftTestCase[]): TestCase[] {
  return drafts
    .filter((tc) => tc.prompt.trim())
    .map((tc, i) => ({
      id: `tc-${i}`,
      prompt: tc.prompt.trim(),
      assertions: tc.assertions.filter((a) => a.value.trim()).map((a) => ({ type: a.type, value: a.value.trim() })),
    }))
}

interface TestCaseFieldsProps {
  testCases: DraftTestCase[]
  onChange: Dispatch<SetStateAction<DraftTestCase[]>>
}

// TestCaseFields renders the repeated "test case card" editing UI (prompt +
// assertion rows), shared between creating a new evaluation and editing an
// existing one's test cases.
export function TestCaseFields({ testCases, onChange }: TestCaseFieldsProps) {
  const updateTestCase = (i: number, patch: Partial<DraftTestCase>) => {
    onChange((prev) => prev.map((tc, idx) => (idx === i ? { ...tc, ...patch } : tc)))
  }

  const updateAssertion = (tcIndex: number, aIndex: number, patch: Partial<DraftAssertion>) => {
    onChange((prev) =>
      prev.map((tc, idx) =>
        idx === tcIndex
          ? { ...tc, assertions: tc.assertions.map((a, ai) => (ai === aIndex ? { ...a, ...patch } : a)) }
          : tc,
      ),
    )
  }

  const addAssertion = (tcIndex: number) => {
    onChange((prev) =>
      prev.map((tc, idx) =>
        idx === tcIndex ? { ...tc, assertions: [...tc.assertions, { type: 'contains', value: '' }] } : tc,
      ),
    )
  }

  const removeAssertion = (tcIndex: number, aIndex: number) => {
    onChange((prev) =>
      prev.map((tc, idx) => (idx === tcIndex ? { ...tc, assertions: tc.assertions.filter((_, ai) => ai !== aIndex) } : tc)),
    )
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
          {tc.assertions.map((a, aIndex) => (
            <div className="assertion-row" key={aIndex}>
              <select
                value={a.type}
                onChange={(e) => updateAssertion(tcIndex, aIndex, { type: e.target.value as AssertionType })}
              >
                <option value="contains">contains</option>
                <option value="not_contains">not contains</option>
                <option value="regex">matches regex</option>
              </select>
              <input
                type="text"
                placeholder="value"
                value={a.value}
                onChange={(e) => updateAssertion(tcIndex, aIndex, { value: e.target.value })}
              />
              {tc.assertions.length > 1 && (
                <button type="button" className="danger-button" onClick={() => removeAssertion(tcIndex, aIndex)}>
                  ×
                </button>
              )}
            </div>
          ))}
          <button type="button" className="button-secondary" onClick={() => addAssertion(tcIndex)}>
            + Assertion
          </button>
        </div>
      ))}
      <button type="button" className="button-secondary" onClick={addTestCase}>
        + Test case
      </button>
    </>
  )
}
