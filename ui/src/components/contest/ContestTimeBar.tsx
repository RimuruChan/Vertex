import { useEffect, useState } from 'react'
import { useContestSpace } from './ContestContext'

export default function ContestTimeBar() {
  const space = useContestSpace()
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const update = () => setNow(Date.now())
    const timer = window.setInterval(update, 1000)
    document.addEventListener('visibilitychange', update)
    return () => {
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', update)
    }
  }, [])
  const contest = space?.details?.contest
  const start = Date.parse(contest?.beginAt ?? '')
  const end = Date.parse(contest?.endAt ?? '')
  const valid = Number.isFinite(start) && Number.isFinite(end) && end > start
  const progress = valid ? Math.min(100, Math.max(0, ((now - start) / (end - start)) * 100)) : 0
  const running = valid && now >= start && now < end
  const label = !valid
    ? '正在加载比赛时间'
    : now < start
      ? '比赛尚未开始'
      : now >= end
        ? '比赛已结束'
        : `比赛已进行 ${progress.toFixed(1)}%`

  return (
    <div
      className="relative h-0.5 w-full overflow-x-clip bg-primary/10"
      role="progressbar"
      aria-label="比赛时间进度"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={valid ? Number(progress.toFixed(1)) : undefined}
      aria-valuetext={label}
      title={label}
    >
      <div
        key={contest?.id}
        className="relative h-full bg-gradient-to-r from-primary/35 to-primary/80 transition-[width] duration-1000 ease-linear motion-reduce:transition-none"
        style={{
          width: `${progress}%`,
        }}
      >
        {running && progress > 0 && (
          <span
            aria-hidden="true"
            className="absolute -top-1 right-0 h-2.5 w-10 translate-x-1/2"
            style={{
              background:
                'radial-gradient(ellipse at center, var(--primary) 0%, color-mix(in srgb, var(--primary) 70%, transparent) 20%, transparent 72%)',
            }}
          >
            <span className="absolute left-1/2 top-1 h-0.5 w-3 -translate-x-1/2 rounded-full bg-gradient-to-r from-transparent via-primary to-transparent" />
          </span>
        )}
      </div>
    </div>
  )
}
