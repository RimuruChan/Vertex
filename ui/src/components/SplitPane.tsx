import { useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { DIVIDER_WIDTH, pointerSplit, splitLimits } from '@/lib/splitPane'

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
  const dragRef = useRef<{ pointerId: number; offset: number } | null>(null)
  const [percent, setPercent] = useState(() => {
    try {
      const stored = Number(localStorage.getItem(STORAGE_KEY))
      return Number.isFinite(stored) && stored >= MIN_PERCENT && stored <= MAX_PERCENT ? stored : 50
    } catch {
      return 50
    }
  })
  const [dragging, setDragging] = useState(false)
  const [limits, setLimits] = useState({ min: MIN_PERCENT, max: MAX_PERCENT })

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const observer = new ResizeObserver(() => {
      // On mobile the caller shows a single pane; retain the desktop split.
      if (getComputedStyle(container).display !== 'grid') return
      const next = splitLimits(container.getBoundingClientRect().width)
      setLimits(next)
      setPercent((value) => Math.min(next.max, Math.max(next.min, value)))
    })
    observer.observe(container)
    return () => observer.disconnect()
  }, [])

  function updateFromPointer(clientX: number) {
    const container = containerRef.current
    if (!container) return
    const bounds = container.getBoundingClientRect()
    setPercent(pointerSplit(clientX - bounds.left - (dragRef.current?.offset ?? 0), bounds.width))
  }

  useEffect(() => {
    if (!dragging) return
    const onBlur = () => {
      dragRef.current = null
      setDragging(false)
    }
    window.addEventListener('blur', onBlur)
    // Stops the drag from selecting the statement text underneath.
    const previousUserSelect = document.body.style.userSelect
    const previousCursor = document.body.style.cursor
    document.body.style.userSelect = 'none'
    document.body.style.cursor = 'col-resize'
    return () => {
      window.removeEventListener('blur', onBlur)
      document.body.style.userSelect = previousUserSelect
      document.body.style.cursor = previousCursor
    }
  }, [dragging])

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, String(Math.round(percent)))
    } catch {
      /* Resizing still works without persistence. */
    }
  }, [percent])

  return (
    <div
      ref={containerRef}
      className={cn(
        'flex flex-col lg:grid lg:grid-cols-[var(--split-columns)] lg:grid-rows-[minmax(0,1fr)]',
        className,
      )}
      style={
        {
          '--split-columns': `minmax(0, ${percent}fr) ${DIVIDER_WIDTH}px minmax(0, ${100 - percent}fr)`,
        } as CSSProperties
      }
    >
      <div className={cn('min-h-0 min-w-0 flex-1 flex-col', leftClassName)}>{left}</div>

      <div
        role="separator"
        aria-orientation="vertical"
        aria-valuenow={Math.round(percent)}
        aria-valuemin={Math.ceil(limits.min)}
        aria-valuemax={Math.floor(limits.max)}
        aria-label="调整阅读与代码面板宽度"
        aria-valuetext={percent === 50 ? '左右等宽' : `阅读区域 ${Math.round(percent)}%`}
        title="拖动调整宽度，靠近中间自动吸附；双击或按 Enter 恢复居中"
        tabIndex={0}
        onPointerDown={(event) => {
          if (event.button !== 0) return
          event.preventDefault()
          const bounds = event.currentTarget.getBoundingClientRect()
          dragRef.current = {
            pointerId: event.pointerId,
            offset: event.clientX - bounds.left - bounds.width / 2,
          }
          event.currentTarget.focus()
          event.currentTarget.setPointerCapture(event.pointerId)
          setDragging(true)
        }}
        onPointerMove={(event) => {
          if (dragRef.current?.pointerId === event.pointerId) updateFromPointer(event.clientX)
        }}
        onPointerUp={(event) => {
          if (dragRef.current?.pointerId !== event.pointerId) return
          updateFromPointer(event.clientX)
          dragRef.current = null
          setDragging(false)
          event.currentTarget.releasePointerCapture(event.pointerId)
        }}
        onLostPointerCapture={() => {
          dragRef.current = null
          setDragging(false)
        }}
        onDoubleClick={() => setPercent(50)}
        onKeyDown={(event) => {
          if (!['ArrowLeft', 'ArrowRight', 'Enter', 'Home'].includes(event.key)) return
          event.preventDefault()
          if (event.key === 'ArrowLeft') setPercent((value) => Math.max(limits.min, value - 2))
          else if (event.key === 'ArrowRight')
            setPercent((value) => Math.min(limits.max, value + 2))
          else setPercent(50)
        }}
        className={cn(
          'hidden touch-none cursor-col-resize items-center justify-center rounded bg-background transition-colors lg:flex',
          'hover:bg-accent focus-visible:bg-accent focus-visible:outline-none',
          dragging && 'bg-primary/30',
        )}
      >
        <span
          className={cn(
            'h-8 w-px bg-border',
            dragging && percent === 50 && 'h-12 w-0.5 bg-primary',
          )}
        />
      </div>

      <div className={cn('min-h-0 min-w-0 flex-1 flex-col', rightClassName)}>{right}</div>
    </div>
  )
}
