import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

const MIN_PERCENT = 25
const MAX_PERCENT = 75
const STORAGE_KEY = 'vertex-split'

type SplitPaneProps = {
  left: ReactNode
  right: ReactNode
  className?: string
}

/**
 * Two panes with a draggable divider, remembered across visits. Below `md` the
 * panes stack, because a 50/50 split is unusable on a phone.
 */
export default function SplitPane({ left, right, className }: SplitPaneProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [percent, setPercent] = useState(() => {
    const stored = Number(localStorage.getItem(STORAGE_KEY))
    return Number.isFinite(stored) && stored >= MIN_PERCENT && stored <= MAX_PERCENT ? stored : 50
  })
  const [dragging, setDragging] = useState(false)

  const updateFromPointer = useCallback((clientX: number) => {
    const container = containerRef.current
    if (!container) return
    const bounds = container.getBoundingClientRect()
    const next = ((clientX - bounds.left) / bounds.width) * 100
    setPercent(Math.min(MAX_PERCENT, Math.max(MIN_PERCENT, next)))
  }, [])

  useEffect(() => {
    if (!dragging) return
    const onMove = (event: PointerEvent) => updateFromPointer(event.clientX)
    const onUp = () => setDragging(false)
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    // Stops the drag from selecting the statement text underneath.
    document.body.style.userSelect = 'none'
    document.body.style.cursor = 'col-resize'
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
    }
  }, [dragging, updateFromPointer])

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, String(Math.round(percent)))
  }, [percent])

  return (
    <div ref={containerRef} className={cn('flex flex-col md:flex-row', className)}>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col md:flex-none" style={paneStyle(percent)}>
        {left}
      </div>

      <div
        role="separator"
        aria-orientation="vertical"
        aria-valuenow={Math.round(percent)}
        tabIndex={0}
        onPointerDown={(event) => {
          event.preventDefault()
          setDragging(true)
        }}
        onKeyDown={(event) => {
          if (event.key === 'ArrowLeft') setPercent((value) => Math.max(MIN_PERCENT, value - 2))
          if (event.key === 'ArrowRight') setPercent((value) => Math.min(MAX_PERCENT, value + 2))
        }}
        className={cn(
          'hidden w-1.5 shrink-0 cursor-col-resize items-center justify-center border-x border-border bg-background transition-colors md:flex',
          'hover:bg-accent focus-visible:bg-accent focus-visible:outline-none',
          dragging && 'bg-primary/30',
        )}
      >
        <span className="h-8 w-px bg-border" />
      </div>

      <div className="flex min-h-0 min-w-0 flex-1 flex-col">{right}</div>
    </div>
  )
}

function paneStyle(percent: number) {
  // Only applied at md+ via the flex-none class above; below md the pane is
  // a normal flex child and this basis is ignored by flex-col.
  return { flexBasis: `${percent}%` } as const
}
