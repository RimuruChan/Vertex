import { ArrowUpRight, ChevronDown, CircleAlert } from 'lucide-react'
import { Link } from '@/domain/navigation'
import type { DomainMaterialIssue, DomainTreeEntry } from '@/generated/api/model'
import { entryLabel } from '@/lib/authoring-materials'
import { cn } from '@/lib/utils'
import { checkMaterialTarget } from './check-material-target'

export function CheckIssueGroup({
  title,
  description,
  issues,
  entries,
  problemId,
  historical,
  canEdit,
  blocking = false,
}: {
  title: string
  description: string
  issues: DomainMaterialIssue[]
  entries: DomainTreeEntry[]
  problemId: string
  historical: boolean
  canEdit: boolean
  blocking?: boolean
}) {
  if (!issues.length) return null
  return (
    <details
      open={blocking || undefined}
      className={cn(
        'group overflow-hidden rounded-lg border',
        blocking ? 'border-destructive/25' : 'border-amber-500/25',
      )}
    >
      <summary className="flex cursor-pointer list-none items-center gap-3 px-4 py-3 [&::-webkit-details-marker]:hidden">
        <CircleAlert
          className={cn(
            'size-4 shrink-0',
            blocking ? 'text-destructive' : 'text-amber-600 dark:text-amber-400',
          )}
        />
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium">
            {title} · {issues.length}
          </span>
          <span className="mt-0.5 block text-xs leading-5 text-muted-foreground">
            {description}
          </span>
        </span>
        <ChevronDown className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180" />
      </summary>
      <ul className="max-h-80 divide-y overflow-auto border-t">
        {issues.map((issue, index) => {
          const entry = entries.find((item) => item.id === issue.entryId)
          const target = canEdit && entry ? checkMaterialTarget(problemId, entry) : undefined
          return (
            <li
              key={`${issue.code}-${issue.entryId}-${index}`}
              className="flex flex-wrap items-start gap-3 px-4 py-3"
            >
              <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded bg-muted text-[11px] tabular-nums text-muted-foreground">
                {index + 1}
              </span>
              <div className="min-w-0 flex-1 basis-40">
                <p className="break-words text-sm leading-6">{issue.message}</p>
                {issue.entryId && (
                  <p className="mt-1 break-words text-xs text-muted-foreground">
                    {entry ? entryLabel(entry) : '当前材料中已不存在此项'}
                  </p>
                )}
              </div>
              {target && (
                <Link
                  to={target}
                  className="inline-flex min-h-8 shrink-0 items-center gap-1 rounded px-2 text-xs font-medium text-primary hover:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  {historical ? '查看当前材料' : '去修复'}
                  <ArrowUpRight className="size-3.5" />
                </Link>
              )}
            </li>
          )
        })}
      </ul>
    </details>
  )
}
