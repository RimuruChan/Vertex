import { Medal as MedalIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { Medal } from '@/lib/contest-medals'

export const medalLabels = { gold: '金牌', silver: '银牌', bronze: '铜牌' }
const colors = {
  gold: 'bg-amber-100 text-amber-700 dark:bg-amber-400/15 dark:text-amber-300',
  silver: 'bg-slate-200 text-slate-600 dark:bg-slate-300/15 dark:text-slate-300',
  bronze: 'bg-orange-100 text-orange-800 dark:bg-orange-400/15 dark:text-orange-300',
}
export function MedalBadge({ medal, children }: { medal: Medal; children?: React.ReactNode }) {
  return (
    <span
      title={medalLabels[medal]}
      aria-label={children === undefined ? medalLabels[medal] : undefined}
      className={cn(
        'inline-flex h-6 shrink-0 items-center justify-center gap-1 rounded-md px-1.5 text-xs font-medium tabular-nums',
        colors[medal],
      )}
    >
      <MedalIcon className="size-3.5" />
      {children}
    </span>
  )
}
