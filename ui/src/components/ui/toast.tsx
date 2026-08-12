import { createContext, useCallback, useContext, useMemo, useRef, useState } from 'react'
import type { PropsWithChildren } from 'react'
import { CheckCircle2, CircleAlert, Info, TriangleAlert, X } from 'lucide-react'
import { cn } from '@/lib/utils'

type ToastKind = 'success' | 'error' | 'warning' | 'info'
type Toast = { id: number; kind: ToastKind; message: string }

type ToastApi = Record<ToastKind, (message: string) => void>

const ToastContext = createContext<ToastApi | null>(null)

const DISMISS_AFTER_MS = 4000

const kindStyles: Record<ToastKind, { icon: typeof Info; className: string }> = {
  success: { icon: CheckCircle2, className: 'text-verdict-ac' },
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

  const push = useCallback(
    (kind: ToastKind, message: string) => {
      const id = nextId.current++
      setToasts((current) => [...current, { id, kind, message }])
      window.setTimeout(() => dismiss(id), DISMISS_AFTER_MS)
    },
    [dismiss],
  )

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
      <div className="pointer-events-none fixed inset-x-0 top-3 z-100 flex flex-col items-center gap-2 px-4">
        {toasts.map((toast) => {
          const { icon: Icon, className } = kindStyles[toast.kind]
          return (
            <div
              key={toast.id}
              role="status"
              className="pointer-events-auto flex w-full max-w-md items-start gap-2.5 rounded-lg border border-border bg-popover px-3.5 py-2.5 text-sm shadow-lg"
            >
              <Icon className={cn('mt-0.5 size-4 shrink-0', className)} />
              <span className="flex-1 break-words">{toast.message}</span>
              <button
                type="button"
                onClick={() => dismiss(toast.id)}
                className="rounded-sm text-muted-foreground transition-colors hover:text-foreground"
              >
                <X className="size-3.5" />
                <span className="sr-only">关闭</span>
              </button>
            </div>
          )
        })}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast() {
  const context = useContext(ToastContext)
  if (!context) throw new Error('useToast must be used inside ToastProvider')
  return context
}
