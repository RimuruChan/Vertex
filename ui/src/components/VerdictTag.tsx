import { cn } from '@/lib/utils'

type VerdictStyle = { short: string; label: string; className: string; dot: string }

/**
 * The judge taxonomy, in one place. `short` is what fits in a table cell,
 * `label` is the full wording used in detail views and tooltips.
 */
const verdicts: Record<string, VerdictStyle> = {
  Accepted: {
    short: 'AC',
    label: '通过',
    className: 'bg-verdict-ac-bg text-verdict-ac',
    dot: 'bg-verdict-ac',
  },
  'Wrong Answer': {
    short: 'WA',
    label: '答案错误',
    className: 'bg-verdict-wa-bg text-verdict-wa',
    dot: 'bg-verdict-wa',
  },
  'Time Limit Exceeded': {
    short: 'TLE',
    label: '超出时间限制',
    className: 'bg-verdict-tle-bg text-verdict-tle',
    dot: 'bg-verdict-tle',
  },
  'Memory Limit Exceeded': {
    short: 'MLE',
    label: '超出内存限制',
    className: 'bg-verdict-tle-bg text-verdict-tle',
    dot: 'bg-verdict-tle',
  },
  'Output Limit Exceeded': {
    short: 'OLE',
    label: '超出输出限制',
    className: 'bg-verdict-tle-bg text-verdict-tle',
    dot: 'bg-verdict-tle',
  },
  'Runtime Error': {
    short: 'RE',
    label: '运行时错误',
    className: 'bg-verdict-wa-bg text-verdict-wa',
    dot: 'bg-verdict-wa',
  },
  'Compile Error': {
    short: 'CE',
    label: '编译错误',
    className: 'bg-verdict-err-bg text-verdict-err',
    dot: 'bg-verdict-err',
  },
  'System Error': {
    short: 'SE',
    label: '系统错误',
    className: 'bg-verdict-err-bg text-verdict-err',
    dot: 'bg-verdict-err',
  },
  Pending: {
    short: '排队中',
    label: '排队中',
    className: 'bg-verdict-pending-bg text-verdict-pending',
    dot: 'bg-verdict-pending',
  },
  Judging: {
    short: '评测中',
    label: '评测中',
    className: 'bg-primary/10 text-primary',
    dot: 'bg-primary',
  },
  Skipped: {
    short: '跳过',
    label: '跳过',
    className: 'bg-verdict-pending-bg text-verdict-pending',
    dot: 'bg-verdict-pending',
  },
}

export function verdictStyle(status: string): VerdictStyle {
  return (
    verdicts[status] ?? {
      short: status,
      label: status,
      className: 'bg-verdict-pending-bg text-verdict-pending',
      dot: 'bg-verdict-pending',
    }
  )
}

export function isPendingVerdict(status: string | undefined): boolean {
  return status === 'Pending' || status === 'Judging'
}

type VerdictTagProps = {
  status: string
  /** Use the full wording instead of the table-sized abbreviation. */
  full?: boolean
  className?: string
}

export default function VerdictTag({ status, full = false, className }: VerdictTagProps) {
  const verdict = verdictStyle(status)
  return (
    <span
      title={verdict.label}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium whitespace-nowrap',
        verdict.className,
        className,
      )}
    >
      <span
        className={cn('size-1.5 rounded-full', verdict.dot, status === 'Judging' && 'animate-pulse')}
      />
      {full ? verdict.label : verdict.short}
    </span>
  )
}
