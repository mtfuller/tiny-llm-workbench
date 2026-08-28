import type { ReactNode } from 'react'

// Badge is the small pill used for tags, "prebuilt", version numbers, etc.
// tone="version" keeps the existing `version-badge` styling.
export function Badge({ children, tone }: { children: ReactNode; tone?: 'default' | 'version' }) {
  return <span className={`badge${tone === 'version' ? ' version-badge' : ''}`}>{children}</span>
}

// VersionBadge renders a resource's published version, or "unpublished" for
// version 0 — a ternary that was duplicated in BenchmarkDetail and
// EvaluationDetail.
export function VersionBadge({ version }: { version: number }) {
  return <Badge tone="version">{version === 0 ? 'unpublished' : `v${version}`}</Badge>
}
