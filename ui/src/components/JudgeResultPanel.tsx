import { useState } from 'react'
import { Link } from '@/domain/navigation'
import { ExternalLink, Clock3, ShieldCheck, Info } from 'lucide-react'
import type { DtoSubmissionResponse as Submission } from '@/generated/api/model'
import VerdictTag, { isPendingVerdict, verdictStyle } from '@/components/VerdictTag'
import { formatMemory, formatTime, shortId } from '@/lib/format'
import { cn } from '@/lib/utils'
import { submissionHref } from '@/lib/routes'

/**
 * The result of the submission you just made, shown next to your code instead
 * of on a page you have to navigate to.
 */
export default function JudgeResultPanel({
  submission,
  feedback = 'full',
}: {
  submission: Submission
  feedback?: string
}) {
  const pending = isPendingVerdict(submission.status)
  const hiddenResult = submission.status === 'Submitted' || feedback === 'none'
  const summaryOnly = feedback === 'summary'
  const firstErrorOnly = feedback === 'first_error'
  const firstError = firstErrorOnly ? submission.caseResults?.[0] : undefined
  const total =
    hiddenResult || summaryOnly || firstErrorOnly
      ? 0
      : submission.totalCases || submission.caseResults?.length || 0
  const cases = hiddenResult || summaryOnly || firstErrorOnly ? [] : (submission.caseResults ?? [])
  const passed = cases.filter((item) => item.verdict === 'Accepted').length
  const failed = cases.filter(
    (item) => !['Accepted', 'Pending', 'Judging', 'Skipped'].includes(item.verdict),
  )
  const [onlyFailures, setOnlyFailures] = useState(false)
  const [hovered, setHovered] = useState<number | null>(null)
  const [focused, setFocused] = useState<number | null>(null)
  const visible = onlyFailures && failed.length ? failed : cases
  const active = cases.find((item) => item.caseIndex === (hovered ?? focused)) ?? failed[0]

  return (
    <div className="flex h-full min-h-0 flex-col bg-card" aria-busy={pending}>
      <div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 px-3 pt-2 text-xs">
        <VerdictTag status={hiddenResult && !pending ? 'Submitted' : submission.status} full />
        <span className="text-muted-foreground tabular-nums" role="status">
          {pending
            ? total
              ? `已评测 ${submission.judgedCases} / ${total}`
              : submission.status === 'Pending'
                ? '等待评测机…'
                : '评测进行中…'
            : firstErrorOnly
              ? '仅公布首个失败测试点'
              : cases.length
                ? `通过 ${passed} / ${cases.length}`
                : hiddenResult
                  ? '本场不公开评测结果'
                  : summaryOnly
                    ? '仅公布最终判定'
                    : submission.status === 'Compile Error'
                      ? '编译未通过'
                      : '暂无测试点详情'}
        </span>
        <Link
          to={submissionHref(submission)}
          target="_blank"
          rel="noopener noreferrer"
          title="在新标签页查看提交详情"
          aria-label={`在新标签页查看提交 #${submission.publicId || shortId(submission.id)} 的详情`}
          className="ml-auto inline-flex shrink-0 items-center gap-1 text-primary hover:underline"
        >
          详情 #{submission.publicId || shortId(submission.id)}
          <ExternalLink className="size-3" />
        </Link>
      </div>
      {cases.length > 0 && (
        <div className="flex shrink-0 items-center gap-3 px-3 py-1 text-[11px] text-muted-foreground">
          {cases.length > 0 && (
            <>
              <span className="inline-flex items-center gap-1">
                <i className="size-1.5 rounded-full bg-verdict-ac" />
                通过 {passed}
              </span>
              {failed.length > 0 && (
                <button
                  type="button"
                  aria-pressed={onlyFailures}
                  onClick={() => {
                    setOnlyFailures((value) => !value)
                    setHovered(null)
                    setFocused(null)
                  }}
                  className={cn(
                    'rounded px-1 py-0.5 hover:bg-muted focus-visible:outline focus-visible:outline-primary',
                    onlyFailures && 'bg-muted text-foreground',
                  )}
                >
                  <span className="text-verdict-wa">未通过 {failed.length}</span> ·{' '}
                  {onlyFailures ? '显示全部' : '只看未通过'}
                </button>
              )}
            </>
          )}
          {!pending && submission.status !== 'Submitted' && (
            <span className="ml-auto tabular-nums">
              {formatTime(submission.totalTimeMs)} · {formatMemory(submission.peakMemoryKb)}
            </span>
          )}
        </div>
      )}
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 py-1">
        {firstErrorOnly ? (
          <div className="space-y-3 py-4 text-sm">
            <p className="text-muted-foreground">
              {pending
                ? '评测进行中…'
                : firstError
                  ? '首个失败测试点'
                  : submission.status === 'Accepted'
                    ? '测试通过'
                    : submission.status === 'Compile Error'
                      ? '编译未通过'
                      : '暂无失败测试点信息'}
            </p>
            {firstError && (
              <div className="flex items-center gap-3">
                <span className="font-mono">#{firstError.caseIndex}</span>
                <VerdictTag status={firstError.verdict} full />
              </div>
            )}
            {submission.status === 'Compile Error' && submission.compileResult && (
              <pre className="whitespace-pre-wrap break-words font-mono text-xs">
                {submission.compileResult}
              </pre>
            )}
            <p className="text-xs text-muted-foreground">
              不公开其他测试点、检查器输出及资源用量。
            </p>
          </div>
        ) : hiddenResult || summaryOnly ? (
          <div className="mx-auto flex h-full w-fit max-w-full items-center justify-center gap-3 px-2 py-2">
            <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary">
              <ShieldCheck className="size-4" />
            </span>
            <div className="min-w-0 max-w-sm text-left">
              <p className="text-xs font-medium">
                {hiddenResult
                  ? '提交已收到，赛后公布结果'
                  : pending
                    ? '评测进行中，稍后公布最终判定'
                    : '本场仅公布最终判定'}
              </p>
              <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
                {hiddenResult
                  ? '比赛期间不展示判定、得分及测试点信息。你可以继续编写下一份提交。'
                  : '测试点详情、编译信息和资源用量暂不公开，比赛结束后可查看。'}
              </p>
            </div>
          </div>
        ) : submission.status === 'Compile Error' && submission.compileResult ? (
          <pre className="font-mono text-xs text-verdict-err whitespace-pre-wrap break-words">
            {submission.compileResult}
          </pre>
        ) : visible.length > 0 ? (
          <div
            role="group"
            aria-label="测试点结果分布"
            className="grid gap-1"
            style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(24px, 1fr))' }}
          >
            {visible.map((item) => {
              const style = verdictStyle(item.verdict)
              const description = `#${item.caseIndex} ${style.label} · ${formatTime(item.timeMs)} · ${formatMemory(item.memoryKb)}`
              return (
                <button
                  key={item.caseIndex}
                  type="button"
                  aria-label={description}
                  title={description}
                  onMouseEnter={() => setHovered(item.caseIndex)}
                  onMouseLeave={() => setHovered(null)}
                  onFocus={() => setFocused(item.caseIndex)}
                  onBlur={() => setFocused(null)}
                  className={cn(
                    'grid h-5 place-items-center rounded border border-transparent font-mono text-[10px] font-medium tabular-nums transition-colors hover:border-current focus-visible:border-current focus-visible:outline-none',
                    style.className,
                  )}
                >
                  {item.caseIndex}
                </button>
              )
            })}
          </div>
        ) : (
          <div className="mx-auto flex h-full w-fit max-w-full items-center justify-center gap-3 px-2 py-2">
            <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground">
              {pending ? <Clock3 className="size-4" /> : <Info className="size-4" />}
            </span>
            <div className="min-w-0 max-w-sm text-left">
              <p className="text-xs font-medium">
                {pending
                  ? submission.status === 'Pending'
                    ? '提交已进入评测队列'
                    : '正在编译和评测'
                  : '暂未提供测试点详情'}
              </p>
              <p className="mt-1 text-[11px] text-muted-foreground">
                {pending ? '结果将自动更新，无需重复提交。' : '可在新标签页查看完整提交状态。'}
              </p>
            </div>
          </div>
        )}
      </div>
      {cases.length > 0 && (
        <div className="min-h-6 shrink-0 px-3 pb-1 text-[11px] text-muted-foreground tabular-nums">
          {active
            ? `#${active.caseIndex} · ${verdictStyle(active.verdict).label} · ${formatTime(active.timeMs)} · ${formatMemory(active.memoryKb)}`
            : cases.length
              ? '按测试点顺序排列 · 悬停或聚焦查看详情'
              : ''}
        </div>
      )}
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
