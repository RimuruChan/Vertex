import { ChevronDown, CircleAlert } from 'lucide-react'
import type { DomainCompatibilityIssue } from '@/generated/api/model'
import { cn } from '@/lib/utils'

export default function PackageIssues({ issues }: { issues: DomainCompatibilityIssue[] }) {
  const groups = [
    {
      id: 'error',
      title: '需要先修复',
      issues: issues.filter((issue) => issue.severity === 'error'),
    },
    {
      id: 'blocking',
      title: '导入后仍需处理',
      issues: issues.filter((issue) => issue.severity === 'blocking'),
    },
    {
      id: 'warning',
      title: '兼容提醒',
      issues: issues.filter((issue) => issue.severity === 'warning'),
    },
    {
      id: 'info',
      title: '其他说明',
      issues: issues.filter((issue) => !['error', 'blocking', 'warning'].includes(issue.severity)),
    },
  ]
  return (
    <div className="space-y-2">
      {groups
        .filter((group) => group.issues.length)
        .map((group) => (
          <details
            key={group.id}
            open={group.id === 'error' || undefined}
            className="group rounded-lg border"
          >
            <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-3 text-sm [&::-webkit-details-marker]:hidden">
              <CircleAlert
                className={cn(
                  'size-4',
                  group.id === 'error' ? 'text-destructive' : 'text-amber-600 dark:text-amber-400',
                )}
              />
              <span className="flex-1 font-medium">
                {group.title} · {group.issues.length}
              </span>
              <ChevronDown className="size-4 text-muted-foreground transition-transform group-open:rotate-180" />
            </summary>
            <ul className="max-h-72 divide-y overflow-auto border-t">
              {group.issues.map((issue, index) => (
                <li key={`${issue.code}:${index}`} className="px-3 py-3">
                  <p className="text-sm leading-6">{issue.message}</p>
                  {issue.path && (
                    <p className="mt-1 break-all font-mono text-xs text-muted-foreground">
                      {issue.path}
                    </p>
                  )}
                </li>
              ))}
            </ul>
          </details>
        ))}
    </div>
  )
}
