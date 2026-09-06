import { useCallback, useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { cn } from '@/lib/utils'

const MIN_PERCENT = 25
const MAX_PERCENT = 75
const STORAGE_KEY = 'vertex-split'

type SplitPaneProps = {
  left: ReactNode
  right: ReactNode
  className?: string
  leftClassName?: string
  rightClassName?: string
}

/**
 * Two panes with a draggable divider, remembered across visits. The caller
 * controls which pane is visible below `lg`, where a split is too cramped.
 */
export default function SplitPane({
  left,
  right,
  className,
  leftClassName,
  rightClassName,
}: SplitPaneProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [percent, setPercent] = useState(() => {
    try {
      const stored = Number(localStorage.getItem(STORAGE_KEY))
      return Number.isFinite(stored) && stored >= MIN_PERCENT && stored <= MAX_PERCENT ? stored : 50
    } catch {
      return 50
    }
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
    try {
      localStorage.setItem(STORAGE_KEY, String(Math.round(percent)))
    } catch {
      /* Resizing still works without persistence. */
    }
  }, [percent])

  return (
    <div ref={containerRef} className={cn('flex flex-col lg:flex-row', className)}>
      <div
        className={cn(
          'min-h-0 min-w-0 flex-1 flex-col lg:flex-none lg:[flex-basis:var(--split-percent)]',
          leftClassName,
        )}
        style={paneStyle(percent)}
      >
        {left}
      </div>

      <div
        role="separator"
        aria-orientation="vertical"
        aria-valuenow={Math.round(percent)}
        aria-valuemin={MIN_PERCENT}
        aria-valuemax={MAX_PERCENT}
        aria-label="调整阅读与代码面板宽度"
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
          'hidden w-3 shrink-0 cursor-col-resize items-center justify-center rounded bg-background transition-colors lg:flex',
          'hover:bg-accent focus-visible:bg-accent focus-visible:outline-none',
          dragging && 'bg-primary/30',
        )}
      >
        <span className="h-8 w-px bg-border" />
      </div>

      <div className={cn('min-h-0 min-w-0 flex-1 flex-col', rightClassName)}>{right}</div>
    </div>
  )
}

function paneStyle(percent: number) {
  // The custom property only becomes flex-basis at the lg breakpoint.
  return { '--split-percent': `${percent}%` } as CSSProperties
}
