import { Link } from '@/domain/navigation'
import { ExternalLink } from 'lucide-react'
import type { DtoSubmissionResponse as Submission } from '@/generated/api/model'
import VerdictTag, { isPendingVerdict, verdictStyle } from '@/components/VerdictTag'
import { Progress } from '@/components/ui/misc'
import { formatMemory, formatTime, shortId } from '@/lib/format'
import { cn } from '@/lib/utils'

/**
 * The result of the submission you just made, shown next to your code instead
 * of on a page you have to navigate to.
 */
export default function JudgeResultPanel({ submission }: { submission: Submission }) {
  const pending = isPendingVerdict(submission.status)
  const total = submission.totalCases || submission.caseResults?.length || 0
  const judged = pending ? submission.judgedCases : total
  const percent = total > 0 ? Math.round((judged / total) * 100) : pending ? 0 : 100

  return (
    <div
      className="flex flex-col gap-3 border-t border-border bg-card px-4 py-3"
      role="status"
      aria-live="polite"
      aria-busy={pending}
    >
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-muted-foreground">
        <VerdictTag status={submission.status} full />
        {pending ? (
          <span className="tabular-nums">
            {total > 0 ? `已评测 ${judged} / ${total} 个测试点` : '等待评测机…'}
          </span>
        ) : (
          <>
            <span>用时 {formatTime(submission.totalTimeMs)}</span>
            <span>内存 {formatMemory(submission.peakMemoryKb)}</span>
          </>
        )}
        <Link
          to={`/submissions/${submission.publicId || submission.id}`}
          className="ml-auto inline-flex items-center gap-1 text-primary hover:underline"
        >
          #{shortId(submission.id)}
          <ExternalLink className="size-3" />
        </Link>
      </div>

      {pending ? <Progress value={percent} aria-label="判题进度" /> : null}

      {submission.status === 'Compile Error' && submission.compileResult ? (
        <pre className="max-h-40 overflow-auto rounded-md border border-border bg-muted p-3 font-mono text-xs whitespace-pre-wrap">
          {submission.compileResult}
        </pre>
      ) : null}

      {submission.caseResults?.length ? <CaseStrip submission={submission} /> : null}
    </div>
  )
}

/** One square per test case — the fastest read on where a solution broke. */
export function CaseStrip({ submission }: { submission: Submission }) {
  return (
    <div className="flex flex-wrap gap-1">
      {submission.caseResults?.map((item) => {
        const style = verdictStyle(item.verdict)
        return (
          <span
            key={item.caseIndex}
            title={`#${item.caseIndex} ${style.label} · ${formatTime(item.timeMs)} · ${formatMemory(item.memoryKb)}`}
            role="img"
            aria-label={`测试点 ${item.caseIndex}：${style.label}，${formatTime(item.timeMs)}，${formatMemory(item.memoryKb)}`}
            className={cn(
              'grid h-6 min-w-6 place-items-center rounded px-1 font-mono text-[11px] font-medium tabular-nums',
              style.className,
            )}
          >
            {item.caseIndex}
          </span>
        )
      })}
    </div>
  )
}
