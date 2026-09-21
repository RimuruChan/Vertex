import { useMemo } from 'react'
import { lineDiff } from '@/lib/line-diff'

/** Source text stays literal: Markdown markup is itself part of the change. */
export default function ReviewTextDiff({ before, after }: { before: string; after: string }) {
  const diff = useMemo(() => lineDiff(before, after), [before, after])
  const added = diff.lines.filter((line) => line.kind === 'added').length
  const removed = diff.lines.filter((line) => line.kind === 'removed').length
  return (
    <div className="min-w-0 overflow-hidden rounded-lg border">
      <div className="flex flex-wrap gap-4 border-b bg-muted/20 px-3 py-2 text-xs">
        <span className="text-green-800 dark:text-green-300">+{added} 新增</span>
        <span className="text-red-800 dark:text-red-300">−{removed} 删除</span>
        <span className="text-muted-foreground">左侧行号：修改前 / 修改后</span>
      </div>
      <div className="max-h-[30rem] overflow-auto" tabIndex={0} aria-label="逐行文本差异">
        {diff.lines.map((line, i) => (
          <div
            key={i}
            className={`grid grid-cols-[2.5rem_2.5rem_1.5rem_minmax(0,1fr)] font-mono text-xs leading-6 ${line.kind === 'added' ? 'bg-green-100/80 text-green-950 dark:bg-green-500/15 dark:text-green-100' : line.kind === 'removed' ? 'bg-red-100/80 text-red-950 dark:bg-red-500/15 dark:text-red-100' : 'text-muted-foreground'}`}
          >
            <span
              aria-hidden="true"
              className="select-none border-r border-current/10 pr-2 text-right opacity-60"
            >
              {line.before}
            </span>
            <span
              aria-hidden="true"
              className="select-none border-r border-current/10 pr-2 text-right opacity-60"
            >
              {line.after}
            </span>
            <span className="select-none text-center font-semibold">
              {line.kind === 'added' ? '+' : line.kind === 'removed' ? '−' : ' '}
            </span>
            <code className="whitespace-pre-wrap break-words pr-3 [overflow-wrap:anywhere]">
              {line.text || '\u00a0'}
            </code>
          </div>
        ))}
      </div>
      {(diff.truncated || diff.coarse) && (
        <p className="border-t p-2 text-xs text-muted-foreground">
          {diff.truncated ? '内容较长，仅显示部分差异。' : '大段更改按整段显示。'}
        </p>
      )}
    </div>
  )
}
