import { SiteMenu } from '@/components/SiteMenu'
import { lazy, Suspense, useEffect, useLayoutEffect, useRef, type PropsWithChildren } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { Link, NavLink, useNavigate } from '@/domain/navigation'
import {
  Code2,
  PanelsTopLeft,
  ListChecks,
  ListTree,
  LogOut,
  Settings,
  Trophy,
  User,
} from 'lucide-react'
import { useAuth } from '@/auth/AuthContext'
import AppearanceMenu, { AppearanceSection } from '@/components/AppearanceMenu'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useToast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { useOptionalDomain } from '@/domain/DomainContext'
import DomainSwitcher from '@/domain/DomainSwitcher'
import VertexLogo from '@/components/VertexLogo'
import { relativeDomainPath } from '@/domain/paths'
import { useContestSpace } from '@/components/contest/ContestContext'
import { ContestIdentity, ContestNavigation } from '@/components/contest/ContestNavigation'
import ContestTimeBar from '@/components/contest/ContestTimeBar'
import { ContestMenu } from '@/components/contest/ContestMenu'

const MockMenu =
  import.meta.env.VITE_MOCK === 'true' ? lazy(() => import('@/mocks/MockMenu')) : null

const navigation = [
  { to: '/problems', label: '题库', icon: Code2, end: false },
  { to: '/problem-sets', label: '题单', icon: ListTree, end: false },
  { to: '/contests', label: '比赛', icon: Trophy, end: false },
  { to: '/submissions', label: '提交记录', icon: ListChecks, end: false },
]

export default function App({ children }: PropsWithChildren) {
  const contestSpace = useContestSpace()
  const { user, ready, logout } = useAuth()
  const domain = useOptionalDomain()
  const navigate = useNavigate()
  const location = useLocation()
  const pathname = relativeDomainPath(location.pathname)
  const toast = useToast()
  const shellRef = useRef<HTMLDivElement>(null)
  const headerRef = useRef<HTMLElement>(null)
  const inWorkbench = pathname.startsWith('/workspace') || pathname.startsWith('/authoring')
  const workspace =
    !!contestSpace ||
    /^\/problems\/[^/]+$/.test(pathname) ||
    /^\/contests\/[^/]+\/problems\/[^/]+$/.test(pathname) ||
    /^\/authoring\/[^/]+$/.test(pathname) ||
    /^\/contests\/[^/]+\/jury$/.test(pathname)

  useLayoutEffect(() => {
    const header = headerRef.current
    if (!header) return
    // Expanded mobile navigation changes the header height; workspace panes
    // subtract its actual size, including borders and accessibility scaling.
    const resize = () =>
      document.documentElement.style.setProperty(
        '--app-header-height',
        `${header.getBoundingClientRect().height}px`,
      )
    resize()
    const observer = new ResizeObserver(resize)
    observer.observe(header)
    return () => {
      observer.disconnect()
      document.documentElement.style.removeProperty('--app-header-height')
    }
  }, [])

  useEffect(() => {
    let label: string | undefined
    if (pathname === '/login') label = '登录'
    else if (pathname === '/domains') label = '浏览域'
    else if (pathname.startsWith('/settings')) label = '域设置'
    else if (pathname.startsWith('/groups')) label = '群组'
    else if (pathname.startsWith('/editorials')) label = '题解'
    else if (pathname.startsWith('/announcements')) label = '公告'
    else if (inWorkbench) label = '工作台'
    else if (/^\/users\//.test(pathname)) label = '个人主页'
    else if (/^\/contests\/[^/]+\/jury$/.test(pathname)) label = '裁判台'
    else if (pathname.startsWith('/admin')) label = '站点管理'
    else {
      label = navigation.find((item) =>
        item.end ? pathname === item.to : pathname.startsWith(item.to),
      )?.label
    }
    document.title = [label, domain?.domain.official ? undefined : domain?.domain.name, 'Vertex']
      .filter(Boolean)
      .join(' · ')
  }, [pathname, inWorkbench, domain?.domain.name, domain?.domain.official])

  async function handleLogout() {
    await logout().catch(() => undefined)
    toast.success('已退出登录')
    navigate('/d/official')
  }

  return (
    <div ref={shellRef} className="flex min-h-screen flex-col bg-background">
      <a
        href="#main-content"
        className="fixed left-3 top-3 z-[100] -translate-y-20 border border-border bg-background px-3 py-2 text-sm focus:translate-y-0"
      >
        跳到主要内容
      </a>
      <header
        ref={headerRef}
        className="sticky top-0 z-40 border-b border-border bg-card/95 backdrop-blur-sm"
      >
        <div className="site-container flex min-h-16 items-center gap-2 sm:gap-4">
          <DomainSwitcher />
          {contestSpace && (
            <div className="hidden min-w-0 flex-1 lg:block">
              <ContestIdentity />
            </div>
          )}
          {!contestSpace && (
            <nav className="hidden shrink-0 items-center gap-1 lg:flex" aria-label="主导航">
              {navigation.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.end}
                  className={({ isActive }) =>
                    cn(
                      'inline-flex items-center gap-1.5 rounded-md px-3 py-2 text-sm transition-colors',
                      isActive
                        ? 'bg-primary/8 font-medium text-primary'
                        : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                    )
                  }
                >
                  {item.label}
                </NavLink>
              ))}
            </nav>
          )}

          <div className="ml-auto flex shrink-0 items-center gap-0 sm:gap-1.5">
            {user && !contestSpace && (
              <Button
                variant="ghost"
                size="sm"
                className={cn(
                  'hidden lg:inline-flex',
                  inWorkbench ? 'bg-primary/8 text-primary' : 'text-muted-foreground',
                )}
                asChild
              >
                <Link to="/workspace" aria-current={inWorkbench ? 'page' : undefined}>
                  <PanelsTopLeft className="size-4" />
                  工作台
                </Link>
              </Button>
            )}
            {!ready ? (
              <span
                role="status"
                aria-label="正在恢复登录"
                className="size-8 animate-pulse rounded-full bg-muted"
              />
            ) : user ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="gap-2"
                    aria-label={`账户：${user.username}`}
                  >
                    <span className="grid size-6 place-items-center rounded-full bg-primary/15 text-xs font-semibold text-primary">
                      {user.username.slice(0, 1).toUpperCase()}
                    </span>
                    <span className="hidden max-w-28 truncate xl:inline">{user.username}</span>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuLabel>{user.username}</DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem asChild>
                    <Link to={`/users/${user.username}`}>
                      <User />
                      个人主页
                    </Link>
                  </DropdownMenuItem>
                  {workspace && (
                    <>
                      <DropdownMenuSeparator />
                      <AppearanceSection />
                    </>
                  )}
                  {user.role === 'admin' ? (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem asChild>
                        <Link to="/admin">
                          <Settings />
                          站点管理
                        </Link>
                      </DropdownMenuItem>
                    </>
                  ) : null}
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onSelect={handleLogout}>
                    <LogOut />
                    退出登录
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
              <>
                <Button size="sm" asChild>
                  <Link to="/login" state={{ from: location.pathname + location.search }}>
                    登录 / 注册
                  </Link>
                </Button>
                {workspace && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon-sm" aria-label="访客选项">
                        <User />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <AppearanceSection />
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </>
            )}
            {contestSpace && <ContestMenu />}
            {!contestSpace && (
              <SiteMenu items={navigation} signedIn={!!user} inWorkbench={inWorkbench} />
            )}
          </div>
        </div>

        {contestSpace && (
          <>
            <div className="hidden lg:block">
              <ContestNavigation />
            </div>
            <ContestTimeBar />
          </>
        )}
      </header>

      <main id="main-content" className={cn('flex-1', workspace && 'min-h-0')}>
        <Suspense fallback={<RouteFallback />}>{children ?? <Outlet />}</Suspense>
      </main>

      {workspace ? null : (
        <footer className="mx-auto mt-6 w-full max-w-7xl px-4 sm:px-6">
          <div className="grid min-h-16 grid-cols-2 items-center gap-x-3 gap-y-3 border-t border-border/60 py-4 text-xs text-muted-foreground md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] md:gap-x-6">
            <div className="flex min-w-0 items-center gap-3 justify-self-start">
              <VertexLogo className="opacity-70 [&_svg]:size-4 [&_span]:text-sm" />
              <span className="hidden whitespace-nowrap border-l border-border pl-3 sm:inline">
                Online Judge
              </span>
            </div>
            <p
              lang="en"
              className="col-span-2 row-start-2 flex items-center justify-center gap-1 whitespace-nowrap text-[11px] tracking-wide md:col-span-1 md:col-start-2 md:row-start-1"
            >
              Made with{' '}
              <span role="img" aria-label="love" className="mx-0.5 text-sm leading-none">
                🩷
              </span>{' '}
              by <span className="font-medium text-foreground/70">Sprite</span>
            </p>
            <div className="col-start-2 row-start-1 flex items-center gap-1 justify-self-end md:col-start-3">
              <AppearanceMenu />
              {MockMenu && (
                <Suspense fallback={null}>
                  <MockMenu placement="footer" />
                </Suspense>
              )}
            </div>
          </div>
        </footer>
      )}
      {MockMenu && workspace ? (
        <Suspense fallback={null}>
          <MockMenu />
        </Suspense>
      ) : null}
    </div>
  )
}

function RouteFallback() {
  return (
    <div
      className="mx-auto flex min-h-64 w-full max-w-7xl items-center justify-center px-4 text-sm text-muted-foreground"
      role="status"
    >
      正在打开页面…
    </div>
  )
}
