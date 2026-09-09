import { useEffect, useState } from 'react'
import { Menu } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { ContestIdentity, ContestNavigation } from './ContestNavigation'

export function ContestMenu() {
  const [open, setOpen] = useState(false)
  useEffect(() => {
    const desktop = window.matchMedia('(min-width: 1024px)')
    const closeOnDesktop = () => {
      if (desktop.matches) setOpen(false)
    }
    desktop.addEventListener('change', closeOnDesktop)
    return () => desktop.removeEventListener('change', closeOnDesktop)
  }, [])
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="icon" className="lg:hidden" aria-label="打开比赛导航">
          <Menu />
        </Button>
      </DialogTrigger>
      <DialogContent side="left" className="gap-6 pb-[max(1.5rem,env(safe-area-inset-bottom))]">
        <DialogTitle className="sr-only">比赛导航</DialogTitle>
        <DialogDescription className="sr-only">
          查看当前比赛信息，切换赛场、提交、榜单与管理页面。
        </DialogDescription>
        <div className="pt-1">
          <ContestIdentity drawer onNavigate={() => setOpen(false)} />
        </div>
        <ContestNavigation vertical onNavigate={() => setOpen(false)} />
      </DialogContent>
    </Dialog>
  )
}
