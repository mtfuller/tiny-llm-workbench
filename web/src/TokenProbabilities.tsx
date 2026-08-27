import { useState, type CSSProperties, type FormEvent } from 'react'
import { getTokenProbabilities, type TokenPosition } from './api'

interface TokenProbabilitiesProps {
  modelName: string
}

const DEFAULT_MAX_TOKENS = 20
const TOP_N = 10

function probability(logprob: number): number {
  return Math.exp(logprob)
}

// TokenProbabilities visualizes a model's "thought process": generate a
// short completion, then for any generated token, show the top candidate
// tokens the model considered at that step and how confident it was in
// each. Each generated token's chip is itself shaded by its own
// probability, so a low-confidence stretch of text is visible at a glance
// before drilling into any single step.
function TokenProbabilities({ modelName }: TokenProbabilitiesProps) {
  const [prompt, setPrompt] = useState('')
  const [maxTokens, setMaxTokens] = useState(DEFAULT_MAX_TOKENS)
  const [positions, setPositions] = useState<TokenPosition[] | null>(null)
  const [selected, setSelected] = useState(0)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!prompt.trim() || running) return

    setRunning(true)
    setError(null)
    setPositions(null)
    try {
      const res = await getTokenProbabilities(modelName, prompt.trim(), maxTokens, TOP_N)
      setPositions(res.positions)
      setSelected(0)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setRunning(false)
    }
  }

  const current = positions?.[selected]

  return (
    <>
      <p className="hint">
        Generates a short completion and shows the top {TOP_N} candidate tokens the model considered at
        each step — a look at its "confidence" as it writes.
      </p>
      <form className="form-grid" onSubmit={handleSubmit}>
        <label className="form-grid-span">
          Prompt
          <input
            type="text"
            placeholder="Once upon a time"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
          />
        </label>
        <label>
          Tokens to generate
          <input
            type="number"
            min={1}
            max={50}
            value={maxTokens}
            onChange={(e) => setMaxTokens(Number(e.target.value))}
          />
        </label>
        <button type="submit" className="form-grid-span" disabled={running || !prompt.trim()}>
          {running ? 'Generating…' : 'Generate'}
        </button>
      </form>

      {error && <p className="error">{error}</p>}

      {positions && positions.length === 0 && <p className="hint">No tokens were generated.</p>}

      {positions && positions.length > 0 && (
        <>
          <div className="tokprob-sequence">
            {positions.map((p, i) => {
              const pct = Math.round(probability(p.logprob) * 100)
              return (
                <button
                  type="button"
                  key={i}
                  className={`tokprob-chip${i === selected ? ' tokprob-chip-active' : ''}`}
                  style={{ '--tokprob-confidence': `${pct}%` } as CSSProperties}
                  title={`${pct}% confidence`}
                  onClick={() => setSelected(i)}
                >
                  {p.token}
                </button>
              )
            })}
          </div>

          {current && (
            <div className="tokprob-detail">
              <h4>
                Candidates for step {selected + 1}: <code>{current.token}</code>
              </h4>
              <div className="tokprob-bars">
                {[...current.topCandidates]
                  .sort((a, b) => b.logprob - a.logprob)
                  .map((c, i) => {
                    const pct = probability(c.logprob) * 100
                    return (
                      <div className="tokprob-bar-row" key={i}>
                        <code className="tokprob-bar-token">{c.token}</code>
                        <div className="tokprob-bar-track">
                          <div className="tokprob-bar-fill" style={{ width: `${Math.max(pct, 1)}%` }} />
                        </div>
                        <span className="tokprob-bar-pct">{pct.toFixed(1)}%</span>
                      </div>
                    )
                  })}
              </div>
            </div>
          )}
        </>
      )}
    </>
  )
}

export default TokenProbabilities
