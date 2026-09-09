import { useEffect, useRef, useState, type ComponentProps } from 'react'
import { Button } from './button'
import { cn } from '@/lib/utils'

export function SaveButton({
  loading = false,
  saved,
  children,
  className,
  ...props
}: ComponentProps<typeof Button> & { saved: boolean }) {
  const [phase, setPhase] = useState<'idle' | 'loading' | 'complete' | 'saved'>('idle')
  const started = useRef(0)
  useEffect(() => {
    if (loading) {
      started.current = performance.now()
      setPhase('loading')
      return
    }
    if (!saved) {
      setPhase('idle')
      return
    }
    // Let fast requests finish their visual sequence without delaying the request.
    const delay = Math.max(0, 450 - (performance.now() - started.current))
    const complete = window.setTimeout(() => setPhase('complete'), delay)
    const label = window.setTimeout(() => setPhase('saved'), delay + 260)
    return () => {
      window.clearTimeout(complete)
      window.clearTimeout(label)
    }
  }, [loading, saved])
  return (
    <Button
      {...props}
      disabled={props.disabled || phase === 'loading' || phase === 'complete'}
      aria-busy={loading}
      data-save-phase={phase}
      className={cn('save-button min-w-32', phase !== 'idle' && 'disabled:opacity-100', className)}
    >
      <span aria-hidden="true" className="save-button-icon">
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="size-5"
        >
          <circle cx="12" cy="12" r="9" pathLength="1" />
          <path d="m8 12 3 3 5-6" pathLength="1" />
        </svg>
      </span>
      <span className="grid" aria-live="polite" aria-atomic="true">
        <span
          className="save-button-label col-start-1 row-start-1"
          style={{ opacity: phase === 'idle' || phase === 'loading' ? 1 : 0 }}
          aria-hidden={phase === 'complete' || phase === 'saved'}
        >
          {phase === 'idle' ? children : '保存中'}
        </span>
        <span
          className="save-button-label col-start-1 row-start-1"
          style={{ opacity: phase === 'saved' ? 1 : 0 }}
          aria-hidden={phase !== 'saved'}
        >
          已保存
        </span>
      </span>
    </Button>
  )
}
