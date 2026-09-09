import { useEffect, useId, useState } from 'react'
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
    <ContestProgressBar
      key={contest?.id}
      progress={progress}
      running={running}
      label={label}
      valid={valid}
    />
  )
}

export function ContestProgressBar({
  progress,
  running,
  label,
  valid = true,
}: {
  progress: number
  running: boolean
  label: string
  valid?: boolean
}) {
  const gradientId = useId()
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
        className="relative h-full rounded-r-full bg-gradient-to-r from-[#cbd7e8] to-[#8ea7c9] transition-[width] duration-1000 ease-linear motion-reduce:transition-none dark:from-primary/35 dark:to-primary/65"
        style={{
          width: `${progress}%`,
        }}
      >
        {running && progress > 0 && (
          <svg
            aria-hidden="true"
            viewBox="0 0 64 10"
            preserveAspectRatio="none"
            className="pointer-events-none absolute -right-1.5 -top-1 h-2.5 w-16 overflow-visible text-[#4774b5] [--progress-tail:#7397c8] dark:text-[#f1f6ff] dark:[--progress-tail:var(--primary)]"
            style={{ maxWidth: 'calc(100% + 6px)' }}
          >
            <defs>
              <linearGradient id={gradientId} x1="0" y1="0" x2="1" y2="0">
                <stop offset="0" stopColor="var(--progress-tail)" stopOpacity="0" />
                <stop offset="0.45" stopColor="var(--progress-tail)" stopOpacity="0.12" />
                <stop offset="0.8" stopColor="var(--progress-tail)" stopOpacity="0.65" />
                <stop offset="1" stopColor="currentColor" />
              </linearGradient>
            </defs>
            <path
              d="M0 5 C24 5 40 2.8 53 2.8 C56 2.8 58 3.5 58 5 C58 6.5 56 7.2 53 7.2 C40 7.2 24 5 0 5Z"
              fill={`url(#${gradientId})`}
              className="opacity-0 blur-[2px] dark:opacity-75"
            />
            <path
              d="M0 5 C24 5 40 3.5 53 3.5 C56 3.5 58 4 58 5 C58 6 56 6.5 53 6.5 C40 6.5 24 5 0 5Z"
              fill={`url(#${gradientId})`}
            />
            <g className="contest-progress-particles" fill="currentColor">
              <circle
                cy="4.5"
                r="0.7"
                style={{ animationDuration: '2.4s', animationDelay: '-0.5s' }}
              />
              <circle
                cy="5.8"
                r="0.5"
                style={{ animationDuration: '3.1s', animationDelay: '-1.8s' }}
              />
              <circle
                cy="5"
                r="0.6"
                style={{ animationDuration: '2.8s', animationDelay: '-2.5s' }}
              />
              <circle
                cy="5.5"
                r="0.55"
                style={{ animationDuration: '2.6s', animationDelay: '-0.9s' }}
              />
              <circle
                cy="4.7"
                r="0.45"
                style={{ animationDuration: '3.3s', animationDelay: '-2s' }}
              />
            </g>
          </svg>
        )}
      </div>
    </div>
  )
}
