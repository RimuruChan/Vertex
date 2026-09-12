import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { MedalConfig } from '@/lib/contest-medals'
import { MedalBadge, medalLabels } from './MedalBadge'

export function ContestMedalSettings({
  value = { mode: 'none', gold: 0, silver: 0, bronze: 0 },
  disabled,
  onChange,
}: {
  value: MedalConfig
  disabled: boolean
  onChange: (value: MedalConfig) => void
}) {
  const percentage = value.mode === 'percentage'
  const overLimit = percentage && value.gold + value.silver + value.bronze > 100
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-3 gap-2" role="group" aria-label="奖牌分配方式">
        {(
          [
            ['none', '不设奖牌'],
            ['count', '固定人数'],
            ['percentage', '人数占比'],
          ] as const
        ).map(([mode, label]) => (
          <Button
            key={mode}
            type="button"
            variant="outline"
            disabled={disabled}
            aria-pressed={value.mode === mode}
            className={
              value.mode === mode
                ? 'border-primary/60 bg-primary/10 text-primary hover:bg-primary/15'
                : ''
            }
            onClick={() => {
              if (mode !== value.mode)
                onChange(
                  mode === 'none'
                    ? { mode, gold: 0, silver: 0, bronze: 0 }
                    : mode === 'percentage'
                      ? { mode, gold: 10, silver: 20, bronze: 30 }
                      : { mode, gold: 1, silver: 2, bronze: 3 },
                )
            }}
          >
            {label}
          </Button>
        ))}
      </div>
      {value.mode !== 'none' && (
        <>
          <div className="grid grid-cols-3 gap-3">
            {(['gold', 'silver', 'bronze'] as const).map((medal) => (
              <div key={medal} className="min-w-0 space-y-2">
                <Label
                  htmlFor={`contest-medal-${medal}`}
                  className="inline-flex items-center gap-1.5"
                >
                  <MedalBadge medal={medal} />
                  {medalLabels[medal]}
                </Label>
                <div className="relative">
                  <Input
                    id={`contest-medal-${medal}`}
                    type="number"
                    min={0}
                    max={percentage ? 100 : 100000}
                    step={1}
                    disabled={disabled}
                    value={value[medal]}
                    className="pr-8 tabular-nums"
                    onChange={(event) =>
                      onChange({ ...value, [medal]: Number(event.target.value) })
                    }
                  />
                  <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                    {percentage ? '%' : '人'}
                  </span>
                </div>
              </div>
            ))}
          </div>
          {overLimit && (
            <p role="alert" className="text-xs text-destructive">
              金银铜比例合计不能超过 100%。
            </p>
          )}
          <p className="text-xs leading-5 text-muted-foreground">
            有效人数为至少通过一道题的选手人数，仅这些选手参与奖牌分配。
            {percentage ? '各档比例分别向下取整。' : '金银铜分别填写人数。'}
            同排名同奖牌，名额边界并列时归入较高档；0 表示该档不设名额。
          </p>
        </>
      )}
    </div>
  )
}
