import { Badge } from './Badge'

// TagCell renders a table cell's worth of tag pills, or an em dash when
// there are none. Used in the Datasets / Knowledge / Benchmark / Evaluation
// row tables.
export default function TagCell({ tags }: { tags?: string[] }) {
  if (!tags || tags.length === 0) return <>—</>
  return (
    <div className="tag-list">
      {tags.map((tag) => (
        <Badge key={tag}>{tag}</Badge>
      ))}
    </div>
  )
}
