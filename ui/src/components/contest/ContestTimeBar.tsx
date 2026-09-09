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
      className="relative h-0.5 w-full bg-primary/10"
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
          maskImage: running
            ? 'linear-gradient(to right, black calc(100% - 8px), transparent)'
            : undefined,
        }}
      >
        {running && progress > 0 && (
          <span
            aria-hidden="true"
            className="absolute -top-1 right-0 h-2.5 w-12 opacity-60 blur-[2px]"
            style={{
              maxWidth: '100%',
              background: 'radial-gradient(ellipse at 65% 50%, var(--primary), transparent 72%)',
            }}
          />
        )}
      </div>
    </div>
  )
}
