import { ChevronRight } from 'lucide-react'
import { Fragment, useMemo } from 'react'
import type { TensorSummary } from './api'

interface ArchitectureDiagramProps {
  tensors: TensorSummary[]
  numLayers: number
  embedding?: TensorSummary
  norm?: TensorSummary
  lmHead?: TensorSummary
  selectedLayer: number
  onSelectLayer: (index: number) => void
  onSelectTensor: (name: string) => void
}

interface LayerGroup {
  start: number
  end: number // inclusive
  count: number
}

function shortTensorName(name: string): string {
  return name.replace(/^model\.layers\.\d+\./, '')
}

// layerSignature captures everything about a layer that matters for
// "is this the same shape as that other layer": every one of its own
// tensors' name/shape/dtype, order-independent.
function layerSignature(tensors: TensorSummary[], layerIndex: number): string {
  return tensors
    .filter((t) => t.block === 'layer' && t.layer === layerIndex)
    .map((t) => `${shortTensorName(t.name)}:${t.shape.join(',')}:${t.dtype}`)
    .sort()
    .join('|')
}

// groupLayers condenses consecutive layers sharing an identical shape
// signature into one group — the common case for a plain transformer stack,
// where all N layers are structurally identical and N separate boxes say
// nothing that one box with a ×N badge doesn't. A model whose layers
// actually vary partway through (e.g. a sliding-window cutover) still gets
// separate groups at the point where the signature changes.
function groupLayers(tensors: TensorSummary[], numLayers: number): LayerGroup[] {
  const groups: LayerGroup[] = []
  let currentSig: string | null = null

  for (let i = 0; i < numLayers; i++) {
    const sig = layerSignature(tensors, i)
    if (currentSig !== null && sig === currentSig) {
      const last = groups[groups.length - 1]
      last.end = i
      last.count++
    } else {
      groups.push({ start: i, end: i, count: 1 })
      currentSig = sig
    }
  }

  return groups
}

// layerColor sweeps a hue gradient (blue → magenta) across depth, so the
// whole stack reads as a small, colorful gradient rather than N identical
// gray boxes — and, as a side effect, makes "how deep is this layer" a
// glanceable property instead of something you have to read a number for.
function layerColor(index: number, total: number): string {
  const t = total > 1 ? index / (total - 1) : 0
  const hue = 200 + t * 120
  return `hsl(${hue}, 62%, 55%)`
}

// ArchitectureDiagram is a compact, non-interactive-layout flow diagram —
// Embedding → every transformer layer in order → Norm → LM Head — showing
// how the whole stack connects, at a glance. It's deliberately a small
// hand-rolled flexbox diagram rather than the app's React Flow agent
// canvas: this has no need for pan/zoom/drag, just a static illustration
// that wraps naturally at any width. Each layer renders as a vertical bar;
// a run of dimensionally-identical layers (the common case) collapses into
// a single bar with a "×N" badge rather than N repeated bars.
function ArchitectureDiagram({
  tensors,
  numLayers,
  embedding,
  norm,
  lmHead,
  selectedLayer,
  onSelectLayer,
  onSelectTensor,
}: ArchitectureDiagramProps) {
  const groups = useMemo(() => groupLayers(tensors, numLayers), [tensors, numLayers])

  return (
    <div className="arch-diagram">
      {embedding && (
        <>
          <button
            type="button"
            className="arch-diagram-node arch-diagram-node-embedding"
            title={embedding.name}
            onClick={() => onSelectTensor(embedding.name)}
          >
            Embedding
          </button>
          <ChevronRight className="arch-diagram-arrow" size={14} />
        </>
      )}

      <div className="arch-diagram-layers">
        {groups.map((group, i) => {
          const isActive = selectedLayer >= group.start && selectedLayer <= group.end
          const mid = (group.start + group.end) / 2
          return (
            <Fragment key={group.start}>
              <button
                type="button"
                className={`arch-diagram-layer-group${isActive ? ' arch-diagram-layer-group-active' : ''}`}
                title={group.count > 1 ? `Layers ${group.start}–${group.end} (identical shapes)` : `Layer ${group.start}`}
                onClick={() => onSelectLayer(group.start)}
              >
                <span className="arch-diagram-node arch-diagram-node-layer" style={{ background: layerColor(mid, numLayers) }} />
                {group.count > 1 && <span className="arch-diagram-layer-badge">×{group.count}</span>}
              </button>
              {i < groups.length - 1 && <ChevronRight className="arch-diagram-arrow" size={12} />}
            </Fragment>
          )
        })}
      </div>

      {norm && (
        <>
          <ChevronRight className="arch-diagram-arrow" size={14} />
          <button
            type="button"
            className="arch-diagram-node arch-diagram-node-norm"
            title={norm.name}
            onClick={() => onSelectTensor(norm.name)}
          >
            Norm
          </button>
        </>
      )}

      <ChevronRight className="arch-diagram-arrow" size={14} />
      <button
        type="button"
        className="arch-diagram-node arch-diagram-node-lmhead"
        title={lmHead ? lmHead.name : 'Tied with token embedding'}
        onClick={() => onSelectTensor(lmHead ? lmHead.name : (embedding?.name ?? ''))}
      >
        LM Head{!lmHead && ' (tied)'}
      </button>
    </div>
  )
}

export default ArchitectureDiagram
