import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { PropsWithChildren } from 'react'
import { CircleAlert, Info, TriangleAlert, X } from 'lucide-react'
import { cn } from '@/lib/utils'

type ToastKind = 'success' | 'error' | 'warning' | 'info'
type Toast = { id: number; kind: ToastKind; message: string }

type ToastApi = Record<ToastKind, (message: string) => void>

const ToastContext = createContext<ToastApi | null>(null)

const DISMISS_AFTER_MS = 4000

const kindStyles: Record<ToastKind, { icon: typeof Info; className: string }> = {
  success: { icon: Info, className: 'text-verdict-ac' },
  error: { icon: CircleAlert, className: 'text-destructive' },
  warning: { icon: TriangleAlert, className: 'text-verdict-tle' },
  info: { icon: Info, className: 'text-muted-foreground' },
}

export function ToastProvider({ children }: PropsWithChildren) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextId = useRef(0)

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id))
  }, [])

  const push = useCallback((kind: ToastKind, message: string) => {
    const id = nextId.current++
    setToasts((current) => [...current, { id, kind, message }])
  }, [])

  const api = useMemo<ToastApi>(
    () => ({
      success: (message) => push('success', message),
      error: (message) => push('error', message),
      warning: (message) => push('warning', message),
      info: (message) => push('info', message),
    }),
    [push],
  )

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div className="pointer-events-none fixed inset-x-0 top-[calc(var(--app-header-height)+1rem)] z-100 flex flex-col items-center gap-2 px-4">
        {toasts.map((toast) => (
          <ToastMessage key={toast.id} toast={toast} onDismiss={dismiss} />
        ))}
      </div>
    </ToastContext.Provider>
  )
}

function ToastMessage({ toast, onDismiss }: { toast: Toast; onDismiss: (id: number) => void }) {
  const [closing, setClosing] = useState(false)
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  useEffect(() => {
    if (closing) {
      const timer = window.setTimeout(() => onDismiss(toast.id), 180)
      return () => window.clearTimeout(timer)
    }
    if (hovered || focused) return
    const timer = window.setTimeout(() => setClosing(true), DISMISS_AFTER_MS)
    return () => window.clearTimeout(timer)
  }, [closing, hovered, focused, toast.id, onDismiss])
  const { icon: Icon, className } = kindStyles[toast.kind]
  return (
    <div
      role={toast.kind === 'error' ? 'alert' : 'status'}
      aria-atomic="true"
      data-state={closing ? 'closed' : 'open'}
      className="motion-toast pointer-events-auto flex w-full max-w-md items-start gap-3 rounded-xl border border-border bg-popover/95 px-4 py-3 text-sm shadow-lg backdrop-blur-sm"
      onPointerEnter={() => setHovered(true)}
      onPointerLeave={() => setHovered(false)}
      onFocusCapture={() => setFocused(true)}
      onBlurCapture={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setFocused(false)
      }}
    >
      {toast.kind === 'success' ? (
        <svg
          aria-hidden="true"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="motion-success mt-0.5 size-5 shrink-0 text-verdict-ac"
        >
          <circle cx="12" cy="12" r="9" pathLength="1" />
          <path d="m8 12 3 3 5-6" pathLength="1" />
        </svg>
      ) : (
        <Icon aria-hidden="true" className={cn('mt-0.5 size-5 shrink-0', className)} />
      )}
      <span className="flex-1 break-words">{toast.message}</span>
      <button
        type="button"
        onClick={() => setClosing(true)}
        className="rounded-md p-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
      >
        <X aria-hidden="true" className="size-4" />
        <span className="sr-only">关闭</span>
      </button>
    </div>
  )
}

export function useToast() {
  const context = useContext(ToastContext)
  if (!context) throw new Error('useToast must be used inside ToastProvider')
  return context
}
