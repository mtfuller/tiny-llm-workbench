import type { TrainingRun } from './api'

function formatDuration(startedAt: string, finishedAt?: string): string {
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now()
  const seconds = Math.max(0, Math.round((end - new Date(startedAt).getTime()) / 1000))
  if (seconds < 60) return `${seconds}s`
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}

// RunStats renders a row of at-a-glance stat tiles for a training run —
// shared by the Training list page's "active run" banner and the run detail
// page's header, so both show the same numbers the same way.
function RunStats({ run }: { run: TrainingRun }) {
  const latest = run.progress[run.progress.length - 1]

  const tiles: { label: string; value: string }[] = [
    { label: 'Iteration', value: latest ? `${latest.iteration} / ${run.config.iterations}` : `0 / ${run.config.iterations}` },
    { label: 'Train loss', value: latest?.trainLoss !== undefined ? latest.trainLoss.toFixed(3) : '—' },
  ]
  if (latest?.valLoss !== undefined) {
    tiles.push({ label: 'Val loss', value: latest.valLoss.toFixed(3) })
  }
  tiles.push({ label: 'Peak mem', value: latest?.peakMemGB !== undefined ? `${latest.peakMemGB.toFixed(1)} GB` : '—' })
  if (latest?.tokensPerSec !== undefined) {
    tiles.push({ label: 'Tokens/sec', value: latest.tokensPerSec.toFixed(0) })
  }
  tiles.push({ label: 'Duration', value: formatDuration(run.startedAt, run.finishedAt) })

  return (
    <div className="stat-row">
      {tiles.map((t) => (
        <div className="stat-tile" key={t.label}>
          <div className="stat-tile-value">{t.value}</div>
          <div className="stat-tile-label">{t.label}</div>
        </div>
      ))}
    </div>
  )
}

export default RunStats
