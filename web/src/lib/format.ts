// Shared display formatters. These were previously copy-pasted into
// BenchmarkDetail, EvaluationDetail, Training, RunStats and elsewhere.

// formatDuration renders the gap between two ISO timestamps as "45s" or
// "2m 13s". finishedAt defaults to now (for a still-running job).
export function formatDuration(startedAt: string, finishedAt?: string): string {
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now()
  const seconds = Math.max(0, Math.round((end - new Date(startedAt).getTime()) / 1000))
  if (seconds < 60) return `${seconds}s`
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}

// formatMs renders a millisecond count as "420ms" or "1.4s".
export function formatMs(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

// formatDate / formatDateTime wrap the browser locale formatters so call
// sites stop juggling `new Date(x).toLocaleDateString()` vs `toLocaleString()`
// inconsistently. A missing value renders as an em dash.
export function formatDate(iso?: string | null): string {
  return iso ? new Date(iso).toLocaleDateString() : '—'
}

export function formatDateTime(iso?: string | null): string {
  return iso ? new Date(iso).toLocaleString() : '—'
}
