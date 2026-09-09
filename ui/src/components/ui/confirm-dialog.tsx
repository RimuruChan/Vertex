import {
  createContext,
  useCallback,
  useContext,
  useRef,
  useState,
  type PropsWithChildren,
} from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

type ConfirmOptions = {
  title: string
  description: string
  confirmLabel?: string
  destructive?: boolean
}

type PendingConfirmation = {
  options: ConfirmOptions
  resolve: (accepted: boolean) => void
}

const ConfirmContext = createContext<((options: ConfirmOptions) => Promise<boolean>) | null>(null)

export function ConfirmProvider({ children }: PropsWithChildren) {
  const [pending, setPending] = useState<PendingConfirmation | null>(null)
  const [displayedOptions, setDisplayedOptions] = useState<ConfirmOptions | null>(null)
  const pendingRef = useRef<PendingConfirmation | null>(null)

  const confirm = useCallback((options: ConfirmOptions) => {
    return new Promise<boolean>((resolve) => {
      pendingRef.current?.resolve(false)
      const request = { options, resolve }
      pendingRef.current = request
      setDisplayedOptions(options)
      setPending(request)
    })
  }, [])

  const settle = useCallback((accepted: boolean) => {
    const current = pendingRef.current
    if (!current) return
    pendingRef.current = null
    setPending(null)
    current.resolve(accepted)
  }, [])

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <Dialog open={Boolean(pending)} onOpenChange={(open) => !open && settle(false)}>
        {displayedOptions ? (
          <DialogContent className="max-w-sm">
            <DialogHeader>
              <DialogTitle>{displayedOptions.title}</DialogTitle>
              <DialogDescription>{displayedOptions.description}</DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => settle(false)}>
                取消
              </Button>
              <Button
                variant={displayedOptions.destructive ? 'destructive' : 'default'}
                onClick={() => settle(true)}
              >
                {displayedOptions.confirmLabel || '确认'}
              </Button>
            </DialogFooter>
          </DialogContent>
        ) : null}
      </Dialog>
    </ConfirmContext.Provider>
  )
}

export function useConfirm() {
  const confirm = useContext(ConfirmContext)
  if (!confirm) throw new Error('useConfirm must be used inside ConfirmProvider')
  return confirm
}
