import { CircleDashed, CircleDot, CircleCheckBig } from 'lucide-react'
import { cn } from '@/lib/utils'

type Props = { status: string | undefined; className?: string }

const labels: Record<string, string> = {
  solved: '已通过',
  attempted: '尝试过，尚未通过',
  none: '未尝试',
}

/**
 * Per-viewer progress marker on the problem list. Shape carries the meaning as
 * well as colour, so it stays readable without relying on hue alone.
 */
export default function ProblemStatusIcon({ status, className }: Props) {
  const label = labels[status ?? 'none'] ?? labels.none
  const Icon =
    status === 'solved' ? CircleCheckBig : status === 'attempted' ? CircleDot : CircleDashed

  return (
    <span title={label} role="img" aria-label={label} className="inline-flex">
      <Icon
        aria-hidden="true"
        className={cn(
          'size-4',
          status === 'solved'
            ? 'text-verdict-ac'
            : status === 'attempted'
              ? 'text-verdict-tle'
              : 'text-muted-foreground/40',
          className,
        )}
      />
    </span>
  )
}
