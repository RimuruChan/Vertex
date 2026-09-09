import { useEffect, useState } from 'react'
import { Menu, PanelsTopLeft, type LucideIcon } from 'lucide-react'
import { Link, NavLink } from '@/domain/navigation'
import { useLocation } from 'react-router-dom'
import { Button } from './ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from './ui/dialog'
import VertexLogo from './VertexLogo'
import { cn } from '@/lib/utils'

export function SiteMenu({
  items,
  signedIn,
  inWorkbench,
}: {
  items: Array<{ to: string; label: string; icon: LucideIcon; end: boolean }>
  signedIn: boolean
  inWorkbench: boolean
}) {
  const [open, setOpen] = useState(false)
  const location = useLocation()
  useEffect(() => setOpen(false), [location.pathname, location.search])
  useEffect(() => {
    const desktop = window.matchMedia('(min-width: 1024px)')
    const close = () => {
      if (desktop.matches) setOpen(false)
    }
    desktop.addEventListener('change', close)
    return () => desktop.removeEventListener('change', close)
  }, [])
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="icon" className="lg:hidden" aria-label="打开主站导航">
          <Menu />
        </Button>
      </DialogTrigger>
      <DialogContent side="left" className="gap-6 pb-[max(1.5rem,env(safe-area-inset-bottom))]">
        <DialogTitle className="sr-only">主站导航</DialogTitle>
        <DialogDescription className="sr-only">
          切换题库、题单、比赛、提交记录和工作台。
        </DialogDescription>
        <Link
          to="/"
          onClick={() => setOpen(false)}
          className="mt-1 flex min-h-9 items-center gap-2 pr-12 font-semibold"
          aria-label="Vertex 首页"
        >
          <VertexLogo />
        </Link>
        <nav aria-label="移动端导航" className="flex flex-col gap-1 border-t border-border pt-4">
          {items.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              onClick={() => setOpen(false)}
              className={({ isActive }) =>
                cn(
                  'flex min-h-11 items-center gap-3 rounded-lg px-4 py-2 text-sm transition-colors',
                  isActive
                    ? 'bg-primary/10 font-medium text-primary'
                    : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                )
              }
            >
              <item.icon className="size-4" />
              {item.label}
            </NavLink>
          ))}
          {signedIn && (
            <div className="mt-3 border-t border-border pt-3">
              <Link
                to="/workspace"
                onClick={() => setOpen(false)}
                aria-current={inWorkbench ? 'page' : undefined}
                className={cn(
                  'flex min-h-11 items-center gap-3 rounded-lg px-4 py-2 text-sm transition-colors',
                  inWorkbench
                    ? 'bg-primary/10 font-medium text-primary'
                    : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                )}
              >
                <PanelsTopLeft className="size-4" />
                工作台
              </Link>
            </div>
          )}
        </nav>
      </DialogContent>
    </Dialog>
  )
}
