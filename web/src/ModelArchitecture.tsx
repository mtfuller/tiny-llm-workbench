import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import ArchitectureDiagram from './ArchitectureDiagram'
import { getModelArchitecture, type Architecture, type TensorSummary } from './api'
import LayerHeatmap from './LayerHeatmap'

interface ModelArchitectureProps {
  modelName: string
}

function formatCount(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(2)}B`
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function formatBytes(n: number): string {
  if (n >= 1024 ** 3) return `${(n / 1024 ** 3).toFixed(2)} GB`
  if (n >= 1024 ** 2) return `${(n / 1024 ** 2).toFixed(1)} MB`
  if (n >= 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${n} B`
}

// shortTensorName drops the "model.layers.N." prefix once a tensor is
// already grouped under that layer's own section, so the per-layer list
// reads as "mlp.down_proj.weight" rather than repeating the layer number.
function shortTensorName(name: string): string {
  return name.replace(/^model\.layers\.\d+\./, '')
}

// ModelArchitecture is a small master-detail widget: a high-level flow
// diagram plus a compact block/tensor nav on the left (derived from the
// model's .safetensors header, not by loading any weights), and a detail
// pane on the right that fills in with a tensor's shape/dtype and weight
// heatmap once you click one — browsing the topology and drilling into a
// specific tensor stay visually separate instead of the page just growing
// taller every time you pick something.
function ModelArchitecture({ modelName }: ModelArchitectureProps) {
  const [arch, setArch] = useState<Architecture | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [layerIndex, setLayerIndex] = useState(0)
  const [selectedTensor, setSelectedTensor] = useState<TensorSummary | null>(null)

  useEffect(() => {
    getModelArchitecture(modelName)
      .then(setArch)
      .catch((err: Error) => setError(err.message))
  }, [modelName])

  const embedding = useMemo(() => arch?.tensors.find((t) => t.block === 'embedding'), [arch])
  const norm = useMemo(() => arch?.tensors.find((t) => t.block === 'norm'), [arch])
  const lmHead = useMemo(() => arch?.tensors.find((t) => t.block === 'lm_head'), [arch])
  const layerTensors = useMemo(
    () => (arch?.tensors ?? []).filter((t) => t.block === 'layer' && t.layer === layerIndex),
    [arch, layerIndex],
  )

  if (error) return <p className="error">{error}</p>
  if (!arch) return <p className="hint">Loading architecture…</p>

  const tensorRow = (t: TensorSummary, label?: string) => (
    <button
      type="button"
      key={t.name}
      className={`arch-nav-row${selectedTensor?.name === t.name ? ' arch-nav-row-active' : ''}`}
      onClick={() => setSelectedTensor(t)}
    >
      <span className="arch-nav-row-name">{label ?? t.name}</span>
      <span className="badge">{t.dtype}</span>
    </button>
  )

  return (
    <>
      <dl className="info-list">
        {arch.modelType && (
          <>
            <dt>Model type</dt>
            <dd>{arch.modelType}</dd>
          </>
        )}
        <dt>Layers</dt>
        <dd>{arch.numLayers}</dd>
        {arch.hiddenSize !== undefined && (
          <>
            <dt>Hidden size</dt>
            <dd>{arch.hiddenSize}</dd>
          </>
        )}
        {arch.vocabSize !== undefined && (
          <>
            <dt>Vocab size</dt>
            <dd>{arch.vocabSize.toLocaleString()}</dd>
          </>
        )}
        <dt>Parameters</dt>
        <dd>{formatCount(arch.numParameters)}</dd>
        <dt>Estimated size</dt>
        <dd>{formatBytes(arch.estimatedBytes)}</dd>
      </dl>

      <ArchitectureDiagram
        tensors={arch.tensors}
        numLayers={arch.numLayers}
        embedding={embedding}
        norm={norm}
        lmHead={lmHead}
        selectedLayer={layerIndex}
        onSelectLayer={setLayerIndex}
        onSelectTensor={(name) => {
          const t = arch.tensors.find((x) => x.name === name)
          if (t) setSelectedTensor(t)
        }}
      />

      <div className="arch-grid">
        <div className="arch-nav">
          {embedding && (
            <div className="arch-block">
              <div className="arch-block-header">Token Embedding</div>
              {tensorRow(embedding)}
            </div>
          )}

          {arch.numLayers > 0 && (
            <div className="arch-block">
              <div className="arch-block-header">
                Transformer Layers × {arch.numLayers}
                <div className="arch-layer-picker">
                  <button
                    type="button"
                    className="icon-button"
                    aria-label="Previous layer"
                    disabled={layerIndex <= 0}
                    onClick={() => setLayerIndex((i) => Math.max(0, i - 1))}
                  >
                    <ChevronLeft size={14} />
                  </button>
                  <span>Layer {layerIndex}</span>
                  <button
                    type="button"
                    className="icon-button"
                    aria-label="Next layer"
                    disabled={layerIndex >= arch.numLayers - 1}
                    onClick={() => setLayerIndex((i) => Math.min(arch.numLayers - 1, i + 1))}
                  >
                    <ChevronRight size={14} />
                  </button>
                </div>
              </div>
              {layerTensors.map((t) => tensorRow(t, shortTensorName(t.name)))}
            </div>
          )}

          {norm && (
            <div className="arch-block">
              <div className="arch-block-header">Final Norm</div>
              {tensorRow(norm)}
            </div>
          )}

          <div className="arch-block">
            <div className="arch-block-header">LM Head</div>
            {lmHead ? tensorRow(lmHead) : <p className="hint arch-tensor-row-muted">Tied with token embedding</p>}
          </div>
        </div>

        <div className="arch-detail">
          {!selectedTensor && <p className="hint arch-detail-empty">Select a tensor on the left to inspect it.</p>}
          {selectedTensor && (
            <>
              <div className="arch-detail-header">
                <code>{selectedTensor.name}</code>
                <span className="badge">{selectedTensor.dtype}</span>
              </div>
              <dl className="info-list">
                <dt>Shape</dt>
                <dd>{selectedTensor.shape.join(' × ')}</dd>
                <dt>Elements</dt>
                <dd>{formatCount(selectedTensor.numElements)}</dd>
              </dl>
              <LayerHeatmap modelName={modelName} tensorName={selectedTensor.name} />
            </>
          )}
        </div>
      </div>
    </>
  )
}

export default ModelArchitecture
