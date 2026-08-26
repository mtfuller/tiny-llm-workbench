// Skeleton renders a shimmering placeholder bar the same shape as the text
// it stands in for, so a loading list/table doesn't flash a bare "Loading…"
// string into an otherwise-empty page.
function Skeleton({ width = '100%', height = '0.9em' }: { width?: string | number; height?: string | number }) {
  return <span className="skeleton" style={{ width, height }} />
}

// TableSkeleton mimics a data-table's shape while a list is loading, so the
// page doesn't jump when real rows arrive. Pass `bare` when the table is
// already inside its own panel (skips the panel-flush wrapper).
export function TableSkeleton({ columns, rows = 3, bare = false }: { columns: number; rows?: number; bare?: boolean }) {
  const table = (
    <table className="data-table">
      <tbody>
        {Array.from({ length: rows }).map((_, r) => (
          <tr key={r}>
            {Array.from({ length: columns }).map((_, c) => (
              <td key={c}>
                <Skeleton width={c === 0 ? '65%' : '45%'} />
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )

  return bare ? table : <div className="panel panel-flush">{table}</div>
}

export default Skeleton
