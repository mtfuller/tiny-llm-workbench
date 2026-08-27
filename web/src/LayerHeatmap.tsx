import { useEffect, useRef, useState } from 'react'
import { getModelHeatmap, type HeatmapData } from './api'

interface LayerHeatmapProps {
  modelName: string
  tensorName: string
}

// readCssColor resolves a CSS custom property (assumed to be a "#rrggbb"
// hex color, true of every theme token this reads) to its RGB triple, so
// the heatmap adapts automatically to the current light/dark theme instead
// of hardcoding colors that would clash with it.
function readCssColor(varName: string): [number, number, number] {
  const raw = getComputedStyle(document.documentElement).getPropertyValue(varName).trim()
  const hex = raw.replace('#', '')
  if (hex.length !== 6) return [128, 128, 128]
  return [parseInt(hex.slice(0, 2), 16), parseInt(hex.slice(2, 4), 16), parseInt(hex.slice(4, 6), 16)]
}

function lerp(a: number, b: number, t: number) {
  return a + (b - a) * t
}

// colorForValue maps a signed weight value to a diverging color: negative
// values fade toward --ok (green), positive toward --accent (the app's own
// brand color) — chosen instead of a generic scientific blue/red scale so
// the heatmap still looks like part of this app, not a bolted-on widget.
function colorForValue(
  v: number,
  maxAbs: number,
  neg: [number, number, number],
  pos: [number, number, number],
  neutral: [number, number, number],
): [number, number, number] {
  if (maxAbs === 0) return neutral
  const t = Math.max(-1, Math.min(1, v / maxAbs))
  const target = t >= 0 ? pos : neg
  const s = Math.abs(t)
  return [lerp(neutral[0], target[0], s), lerp(neutral[1], target[1], s), lerp(neutral[2], target[2], s)]
}

function renderHeatmap(canvas: HTMLCanvasElement, data: HeatmapData) {
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  canvas.width = data.cols
  canvas.height = data.rows

  const image = ctx.createImageData(data.cols, data.rows)
  const maxAbs = Math.max(Math.abs(data.min), Math.abs(data.max)) || 1
  const neg = readCssColor('--ok')
  const pos = readCssColor('--accent')
  const neutral = readCssColor('--surface')

  for (let i = 0; i < data.grid.length; i++) {
    const [r, g, b] = colorForValue(data.grid[i], maxAbs, neg, pos, neutral)
    image.data[i * 4] = r
    image.data[i * 4 + 1] = g
    image.data[i * 4 + 2] = b
    image.data[i * 4 + 3] = 255
  }

  ctx.putImageData(image, 0, 0)
}

// LayerHeatmap fetches and paints a single tensor's weight distribution —
// subsampled to a fixed grid server-side, so even a huge matrix stays a
// small request. Green = negative, the app's accent color = positive,
// intensity = magnitude.
function LayerHeatmap({ modelName, tensorName }: LayerHeatmapProps) {
  const [data, setData] = useState<HeatmapData | null>(null)
  const [error, setError] = useState<string | null>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    setData(null)
    setError(null)
    getModelHeatmap(modelName, tensorName)
      .then(setData)
      .catch((err: Error) => setError(err.message))
  }, [modelName, tensorName])

  useEffect(() => {
    if (data && canvasRef.current) renderHeatmap(canvasRef.current, data)
  }, [data])

  return (
    <div className="heatmap-panel">
      {error && <p className="error">{error}</p>}
      {!error && !data && <p className="hint">Loading…</p>}

      {data && (
        <>
          <canvas ref={canvasRef} className="heatmap-canvas" />
          <div className="heatmap-legend">
            <span>
              <span className="heatmap-swatch heatmap-swatch-neg" /> min {data.min.toFixed(4)}
            </span>
            <span>mean {data.mean.toFixed(4)}</span>
            <span>std {data.std.toFixed(4)}</span>
            <span>
              max {data.max.toFixed(4)} <span className="heatmap-swatch heatmap-swatch-pos" />
            </span>
          </div>
        </>
      )}
    </div>
  )
}

export default LayerHeatmap
