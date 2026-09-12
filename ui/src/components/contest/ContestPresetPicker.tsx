import { Button } from '@/components/ui/button'
import {
  contestFormatDescription,
  contestFormatName,
  contestFormatOptions,
} from '@/lib/contest-formats'
import type { ContestPreset } from './contest-presets'

export function ContestPresetPicker({
  value,
  disabled,
  onApply,
}: {
  value: string
  disabled?: boolean
  onApply: (preset: ContestPreset) => void
}) {
  return (
    <section className="space-y-2" aria-label="赛制">
      <div>
        <h3 className="text-sm font-semibold">赛制</h3>
      </div>
      <div className="grid grid-cols-3 gap-2 sm:grid-cols-5">
        {contestFormatOptions.map(([key]) => (
          <Button
            key={key}
            type="button"
            variant="outline"
            className={
              value === key
                ? 'border-primary/60 bg-primary/10 text-primary hover:bg-primary/15'
                : ''
            }
            aria-pressed={value === key}
            disabled={disabled}
            onClick={() => onApply(key)}
          >
            {contestFormatName(key)}
          </Button>
        ))}
      </div>
      <p className="text-xs leading-5 text-muted-foreground">{contestFormatDescription[value]}</p>
    </section>
  )
}
