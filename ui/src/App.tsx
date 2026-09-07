import {
  lazy,
  Suspense,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type PropsWithChildren,
} from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { Link, NavLink, useNavigate } from '@/domain/navigation'
import {
  BookOpen,
  ChevronDown,
  Code2,
  Hammer,
  LayoutGrid,
  ListChecks,
  ListTree,
  LogOut,
  Menu,
  Monitor,
  Moon,
  Settings,
  Sun,
  Trophy,
  User,
  X,
} from 'lucide-react'
import { useAuth } from '@/auth/AuthContext'
import { useTheme } from '@/components/ThemeProvider'
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
import { relativeDomainPath } from '@/domain/paths'

const MockMenu =
  import.meta.env.VITE_MOCK === 'true' ? lazy(() => import('@/mocks/MockMenu')) : null

const navigation = [
  { to: '/', label: '首页', icon: LayoutGrid, end: true },
  { to: '/problems', label: '题库', icon: Code2, end: false },
  { to: '/problem-sets', label: '题单', icon: ListTree, end: false },
  { to: '/contests', label: '比赛', icon: Trophy, end: false },
  { to: '/submissions', label: '提交', icon: ListChecks, end: false },
  { to: '/editorials', label: '题解', icon: BookOpen, end: false },
  { to: '/authoring', label: '出题', icon: Hammer, end: false },
]

export default function App({ children }: PropsWithChildren) {
  const { user, ready, logout } = useAuth()
  const domain = useOptionalDomain()
  const navigate = useNavigate()
  const location = useLocation()
  const pathname = relativeDomainPath(location.pathname)
  const toast = useToast()
  const [mobileOpen, setMobileOpen] = useState(false)
  const shellRef = useRef<HTMLDivElement>(null)
  const headerRef = useRef<HTMLElement>(null)
  const mobileButtonRef = useRef<HTMLButtonElement>(null)
  const mobileNavRef = useRef<HTMLElement>(null)
  const visibleNavigation = navigation.filter((item) => item.to !== '/authoring' || !!user)
  const workspace =
    /^\/problems\/[^/]+$/.test(pathname) ||
    /^\/contests\/[^/]+\/problems\/[^/]+$/.test(pathname) ||
    /^\/authoring\/[^/]+$/.test(pathname) ||
    /^\/contests\/[^/]+\/jury$/.test(pathname)

  useEffect(() => setMobileOpen(false), [location.pathname])

  useLayoutEffect(() => {
    const header = headerRef.current
    if (!header) return
    // Expanded mobile navigation changes the header height; workspace panes
    // subtract its actual size, including borders and accessibility scaling.
    const resize = () =>
      shellRef.current?.style.setProperty(
        '--app-header-height',
        `${header.getBoundingClientRect().height}px`,
      )
    resize()
    const observer = new ResizeObserver(resize)
    observer.observe(header)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    let label: string | undefined
    if (pathname === '/login') label = '登录'
    else if (/^\/users\//.test(pathname)) label = '个人主页'
    else if (/^\/contests\/[^/]+\/jury$/.test(pathname)) label = '裁判台'
    else if (pathname.startsWith('/admin')) label = '管理后台'
    else {
      label = navigation.find((item) =>
        item.end ? pathname === item.to : pathname.startsWith(item.to),
      )?.label
    }
    document.title = [label, domain?.domain.name, 'Vertex'].filter(Boolean).join(' · ')
  }, [pathname, domain?.domain.name])

  useEffect(() => {
    if (!mobileOpen) return
    mobileNavRef.current?.querySelector<HTMLAnchorElement>('a')?.focus()
    const close = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      setMobileOpen(false)
      window.requestAnimationFrame(() => mobileButtonRef.current?.focus())
    }
    window.addEventListener('keydown', close)
    return () => window.removeEventListener('keydown', close)
  }, [mobileOpen])

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
        <div className="mx-auto flex h-16 w-full max-w-[1440px] items-center gap-2 px-3 sm:gap-4 sm:px-6">
          <DomainSwitcher />
          <nav className="hidden shrink-0 items-center gap-1 lg:flex" aria-label="主导航">
            {visibleNavigation.map((item) => (
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
            {user?.role === 'admin' ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="sm" className="self-center text-muted-foreground">
                    管理
                    <ChevronDown className="size-3.5" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  <DropdownMenuItem asChild>
                    <Link to="/admin">站点管理</Link>
                  </DropdownMenuItem>
                  <DropdownMenuItem asChild>
                    <Link to="/admin/problems">题目管理</Link>
                  </DropdownMenuItem>
                  <DropdownMenuItem asChild>
                    <Link to="/admin/contests">比赛管理</Link>
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
          </nav>

          <div className="ml-auto flex shrink-0 items-center gap-0 sm:gap-1.5">
            {MockMenu ? (
              <Suspense fallback={null}>
                <MockMenu />
              </Suspense>
            ) : null}
            <ThemeToggle />
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
                  <DropdownMenuItem asChild>
                    <Link to="/admin/contests">
                      <Trophy />
                      我的比赛管理
                    </Link>
                  </DropdownMenuItem>
                  {domain && (
                    <>
                      <DropdownMenuItem asChild>
                        <Link to="/groups">
                          <User />
                          域内群组
                        </Link>
                      </DropdownMenuItem>
                      <DropdownMenuItem asChild>
                        <Link to="/settings">
                          <Settings />
                          当前域设置
                        </Link>
                      </DropdownMenuItem>
                    </>
                  )}
                  <DropdownMenuItem asChild>
                    <Link to="/domains">
                      <LayoutGrid />
                      浏览与创建域
                    </Link>
                  </DropdownMenuItem>
                  {user.role === 'admin' ? (
                    <DropdownMenuItem asChild>
                      <Link to="/admin/problems">
                        <Settings />
                        管理后台
                      </Link>
                    </DropdownMenuItem>
                  ) : null}
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onSelect={handleLogout}>
                    <LogOut />
                    退出登录
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
              <Button size="sm" asChild>
                <Link to="/login" state={{ from: location.pathname + location.search }}>
                  登录 / 注册
                </Link>
              </Button>
            )}
            <Button
              ref={mobileButtonRef}
              variant="ghost"
              size="icon"
              className="lg:hidden"
              onClick={() => setMobileOpen((open) => !open)}
              aria-label="切换导航"
              aria-expanded={mobileOpen}
              aria-controls="mobile-navigation"
            >
              {mobileOpen ? <X /> : <Menu />}
            </Button>
          </div>
        </div>

        {mobileOpen ? (
          <nav
            ref={mobileNavRef}
            id="mobile-navigation"
            className="border-t border-border bg-card px-3 py-2 lg:hidden"
            aria-label="移动端导航"
          >
            {visibleNavigation.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                onClick={() => setMobileOpen(false)}
                className={({ isActive }) =>
                  cn(
                    'flex min-h-11 items-center gap-2 rounded-md px-3 py-2 text-sm',
                    isActive ? 'bg-primary/8 font-medium text-primary' : 'text-muted-foreground',
                  )
                }
              >
                <item.icon className="size-4" />
                {item.label}
              </NavLink>
            ))}
            {user?.role === 'admin' ? (
              <>
                <Link
                  to="/admin"
                  onClick={() => setMobileOpen(false)}
                  className="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground"
                >
                  <Settings className="size-4" />
                  站点管理
                </Link>
                <Link
                  to="/admin/problems"
                  onClick={() => setMobileOpen(false)}
                  className="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground"
                >
                  <Code2 className="size-4" />
                  题目管理
                </Link>
                <Link
                  to="/admin/contests"
                  onClick={() => setMobileOpen(false)}
                  className="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground"
                >
                  <Trophy className="size-4" />
                  比赛管理
                </Link>
              </>
            ) : null}
          </nav>
        ) : null}
      </header>

      <main id="main-content" className={cn('flex-1', workspace && 'min-h-0')}>
        <Suspense fallback={<RouteFallback />}>{children ?? <Outlet />}</Suspense>
      </main>

      {workspace ? null : (
        <footer className="mx-auto flex w-full max-w-7xl items-center justify-between gap-3 px-4 py-6 text-xs text-muted-foreground sm:px-6">
          <span className="font-medium">
            vertex <span className="ml-2 font-normal opacity-70">Online Judge</span>
          </span>
          <Link to="/announcements" className="ml-auto hover:text-foreground">
            域公告
          </Link>
          <Link to="/problems" className="hover:text-foreground">
            保持好奇，持续练习。
          </Link>
        </footer>
      )}
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

function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const Icon = theme === 'dark' ? Moon : theme === 'light' ? Sun : Monitor

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="切换主题">
          <Icon />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => setTheme('light')}>
          <Sun />
          浅色
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => setTheme('dark')}>
          <Moon />
          深色
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => setTheme('system')}>
          <Monitor />
          跟随系统
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
