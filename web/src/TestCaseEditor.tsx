import type { Dispatch, SetStateAction } from 'react'
import type { Assertion, AssertionType, VerifyStep } from './api'

export interface DraftAssertion {
  type: AssertionType
  value: string
  // threshold is only used by "similarity" (the minimum required similarity
  // ratio, in (0, 1]).
  threshold?: number
}

const DEFAULT_SIMILARITY_THRESHOLD = 0.85

export function emptyAssertion(): DraftAssertion {
  return { type: 'contains', value: '' }
}

// toDraftAssertions seeds the assertion editor from a saved test case's
// assertions.
export function toDraftAssertions(assertions: Assertion[]): DraftAssertion[] {
  if (assertions.length === 0) return [emptyAssertion()]
  return assertions.map((a) => ({ type: a.type, value: a.value, threshold: a.threshold }))
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

// DraftVerifyStep/emptyVerifyStep/VerifyStepFields are Evaluations-only —
// a Benchmark's test cases have no Environment to verify against.
export interface DraftVerifyStep {
  command: string
  assertions: DraftAssertion[]
}

export function emptyVerifyStep(): DraftVerifyStep {
  return { command: '', assertions: [emptyAssertion()] }
}

// toDraftVerifySteps seeds the verify-step editor from a saved test case's
// verifyCommands.
export function toDraftVerifySteps(steps: VerifyStep[] | undefined): DraftVerifyStep[] {
  return (steps ?? []).map((s) => ({ command: s.command, assertions: toDraftAssertions(s.assertions) }))
}

// toPayloadVerifySteps drops steps with a blank command, ready to attach to
// a test case.
export function toPayloadVerifySteps(drafts: DraftVerifyStep[]): VerifyStep[] {
  return drafts
    .filter((s) => s.command.trim())
    .map((s) => ({ command: s.command.trim(), assertions: toPayloadAssertions(s.assertions) }))
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
// similarity threshold) for a single test case's (or verify step's)
// assertions — shared by every add/edit/generate modal in both Evaluations
// and Benchmarks.
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

interface VerifyStepFieldsProps {
  steps: DraftVerifyStep[]
  onChange: Dispatch<SetStateAction<DraftVerifyStep[]>>
}

// VerifyStepFields renders the repeated "verify step card" editing UI
// (command + nested assertion rows) for a test case's post-turn
// environment-state checks — Evaluations only, mirrors how AssertionFields
// nests inside a test case but one level deeper (each step has its own
// assertions).
export function VerifyStepFields({ steps, onChange }: VerifyStepFieldsProps) {
  const updateStep = (i: number, patch: Partial<DraftVerifyStep>) => {
    onChange((prev) => prev.map((s, idx) => (idx === i ? { ...s, ...patch } : s)))
  }

  const addStep = () => onChange((prev) => [...prev, emptyVerifyStep()])
  const removeStep = (i: number) => onChange((prev) => prev.filter((_, idx) => idx !== i))

  return (
    <>
      {steps.map((step, i) => (
        <div key={i} className="test-case-editor">
          <div className="page-header">
            <strong>Verify step {i + 1}</strong>
            <button type="button" className="danger-button" onClick={() => removeStep(i)}>
              Remove
            </button>
          </div>
          <label>
            Command
            <input
              type="text"
              placeholder="cat /repo/output.txt"
              value={step.command}
              onChange={(e) => updateStep(i, { command: e.target.value })}
            />
          </label>
          <AssertionFields assertions={step.assertions} onChange={(assertions) => updateStep(i, { assertions })} />
        </div>
      ))}
      <button type="button" className="button-secondary" onClick={addStep}>
        + Verify step
      </button>
    </>
  )
}
