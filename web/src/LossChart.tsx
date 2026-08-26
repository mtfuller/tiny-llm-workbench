import { useMemo, useRef, useState } from 'react'
import type { ProgressPoint } from './api'

// Fixed hex (not the theme's --accent/--warn, which shift per light/dark) so
// the train/val pairing stays a validated-safe categorical pair in both
// themes: `node validate_palette.js "#c1633f,#4a86dd" --mode light|dark`
// passes lightness band, chroma floor, CVD separation, and contrast.
const TRAIN_COLOR = '#c1633f'
const VAL_COLOR = '#4a86dd'

const WIDTH = 640
const HEIGHT = 200
const PAD_LEFT = 44
const PAD_RIGHT = 12
const PAD_TOP = 12
const PAD_BOTTOM = 26

interface Point {
  x: number
  y: number
}

function niceTicks(min: number, max: number, count: number): number[] {
  if (min === max) return [min]
  const step = (max - min) / (count - 1)
  return Array.from({ length: count }, (_, i) => min + step * i)
}

function LossChart({ progress }: { progress: ProgressPoint[] }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [hoverIndex, setHoverIndex] = useState<number | null>(null)

  const trainPoints: Point[] = useMemo(
    () =>
      progress
        .filter((p): p is ProgressPoint & { trainLoss: number } => p.trainLoss !== undefined)
        .map((p) => ({ x: p.iteration, y: p.trainLoss })),
    [progress],
  )
  const valPoints: Point[] = useMemo(
    () =>
      progress
        .filter((p): p is ProgressPoint & { valLoss: number } => p.valLoss !== undefined)
        .map((p) => ({ x: p.iteration, y: p.valLoss })),
    [progress],
  )

  if (trainPoints.length === 0) {
    return <p className="hint">No progress data yet.</p>
  }

  const allPoints = [...trainPoints, ...valPoints]
  const xMin = Math.min(...allPoints.map((p) => p.x))
  const xMax = Math.max(...allPoints.map((p) => p.x))
  const yMinRaw = Math.min(...allPoints.map((p) => p.y))
  const yMaxRaw = Math.max(...allPoints.map((p) => p.y))
  const yPad = (yMaxRaw - yMinRaw) * 0.1 || Math.max(yMaxRaw * 0.1, 0.01)
  const yMin = Math.max(0, yMinRaw - yPad)
  const yMax = yMaxRaw + yPad

  const plotWidth = WIDTH - PAD_LEFT - PAD_RIGHT
  const plotHeight = HEIGHT - PAD_TOP - PAD_BOTTOM

  const xScale = (x: number) => PAD_LEFT + ((x - xMin) / (xMax - xMin || 1)) * plotWidth
  const yScale = (y: number) => PAD_TOP + (1 - (y - yMin) / (yMax - yMin || 1)) * plotHeight

  const linePath = (points: Point[]) =>
    points
      .slice()
      .sort((a, b) => a.x - b.x)
      .map((p, i) => `${i === 0 ? 'M' : 'L'}${xScale(p.x).toFixed(1)},${yScale(p.y).toFixed(1)}`)
      .join(' ')

  const yTicks = niceTicks(yMin, yMax, 4)
  const xTicks = niceTicks(xMin, xMax, Math.min(6, xMax - xMin + 1))

  const handleMouseMove = (e: React.MouseEvent<SVGSVGElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    const fracX = (e.clientX - rect.left) / rect.width
    const xValue = xMin + fracX * (xMax - xMin)
    let nearest = 0
    let nearestDist = Infinity
    trainPoints.forEach((p, i) => {
      const dist = Math.abs(p.x - xValue)
      if (dist < nearestDist) {
        nearestDist = dist
        nearest = i
      }
    })
    setHoverIndex(nearest)
  }

  const hovered = hoverIndex !== null ? trainPoints[hoverIndex] : null
  const hoveredVal = hovered ? valPoints.find((p) => p.x === hovered.x) : undefined
  const tooltipLeftPct = hovered ? (xScale(hovered.x) / WIDTH) * 100 : 0

  return (
    <div className="loss-chart" ref={containerRef}>
      {valPoints.length > 0 && (
        <div className="loss-chart-legend">
          <span className="loss-chart-legend-item">
            <span className="loss-chart-swatch" style={{ background: TRAIN_COLOR }} />
            train loss
          </span>
          <span className="loss-chart-legend-item">
            <span className="loss-chart-swatch" style={{ background: VAL_COLOR }} />
            val loss
          </span>
        </div>
      )}
      <div className="loss-chart-plot">
      <svg
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        className="loss-chart-svg"
        onMouseMove={handleMouseMove}
        onMouseLeave={() => setHoverIndex(null)}
        role="img"
        aria-label="Training loss over iterations"
      >
        {yTicks.map((t) => (
          <g key={t}>
            <line x1={PAD_LEFT} x2={WIDTH - PAD_RIGHT} y1={yScale(t)} y2={yScale(t)} className="loss-chart-grid" />
            <text x={PAD_LEFT - 6} y={yScale(t)} className="loss-chart-axis-label" textAnchor="end" dominantBaseline="middle">
              {t.toFixed(2)}
            </text>
          </g>
        ))}
        {xTicks.map((t) => (
          <text
            key={t}
            x={xScale(t)}
            y={HEIGHT - PAD_BOTTOM + 16}
            className="loss-chart-axis-label"
            textAnchor="middle"
          >
            {Math.round(t)}
          </text>
        ))}

        <path d={linePath(trainPoints)} fill="none" stroke={TRAIN_COLOR} strokeWidth={2} strokeLinecap="round" />
        {trainPoints.map((p) => (
          <circle key={`train-${p.x}`} cx={xScale(p.x)} cy={yScale(p.y)} r={2.5} fill={TRAIN_COLOR} />
        ))}

        {valPoints.length > 0 && (
          <>
            <path d={linePath(valPoints)} fill="none" stroke={VAL_COLOR} strokeWidth={2} strokeLinecap="round" />
            {valPoints.map((p) => (
              <circle key={`val-${p.x}`} cx={xScale(p.x)} cy={yScale(p.y)} r={2.5} fill={VAL_COLOR} />
            ))}
          </>
        )}

        {hovered && (
          <>
            <line
              x1={xScale(hovered.x)}
              x2={xScale(hovered.x)}
              y1={PAD_TOP}
              y2={HEIGHT - PAD_BOTTOM}
              className="loss-chart-crosshair"
            />
            <circle cx={xScale(hovered.x)} cy={yScale(hovered.y)} r={4} fill={TRAIN_COLOR} stroke="var(--surface)" strokeWidth={1.5} />
            {hoveredVal && (
              <circle cx={xScale(hoveredVal.x)} cy={yScale(hoveredVal.y)} r={4} fill={VAL_COLOR} stroke="var(--surface)" strokeWidth={1.5} />
            )}
          </>
        )}
      </svg>

      {hovered && (
        <div className="loss-chart-tooltip" style={{ left: `${tooltipLeftPct}%` }}>
          <div className="loss-chart-tooltip-title">Iteration {hovered.x}</div>
          <div>
            <span className="loss-chart-swatch" style={{ background: TRAIN_COLOR }} /> train {hovered.y.toFixed(4)}
          </div>
          {hoveredVal && (
            <div>
              <span className="loss-chart-swatch" style={{ background: VAL_COLOR }} /> val {hoveredVal.y.toFixed(4)}
            </div>
          )}
        </div>
      )}
      </div>
    </div>
  )
}

export default LossChart
