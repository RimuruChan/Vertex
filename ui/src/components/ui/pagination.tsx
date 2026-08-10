import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'

type PaginationProps = {
  page: number
  size: number
  total: number
  onChange: (page: number) => void
}

/** Compact prev/next pager — problem and submission lists are browsed linearly. */
export function Pagination({ page, size, total, onChange }: PaginationProps) {
  const lastPage = Math.max(1, Math.ceil(total / size))
  if (total === 0) return null

  const first = (page - 1) * size + 1
  const last = Math.min(page * size, total)

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border px-5 py-3 text-sm text-muted-foreground">
      <span>
        {first}–{last} / 共 {total} 条
      </span>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="icon-sm"
          disabled={page <= 1}
          onClick={() => onChange(page - 1)}
          aria-label="上一页"
        >
          <ChevronLeft />
        </Button>
        <span className="tabular-nums">
          {page} / {lastPage}
        </span>
        <Button
          variant="outline"
          size="icon-sm"
          disabled={page >= lastPage}
          onClick={() => onChange(page + 1)}
          aria-label="下一页"
        >
          <ChevronRight />
        </Button>
      </div>
    </div>
  )
}
