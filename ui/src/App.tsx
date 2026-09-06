import { Suspense, useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  BookOpen,
  ChevronDown,
  Code2,
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

const navigation = [
  { to: '/', label: '首页', icon: LayoutGrid, end: true },
  { to: '/problems', label: '题库', icon: Code2, end: false },
  { to: '/problem-sets', label: '题单', icon: ListTree, end: false },
  { to: '/contests', label: '比赛', icon: Trophy, end: false },
  { to: '/submissions', label: '提交', icon: ListChecks, end: false },
  { to: '/editorials', label: '题解', icon: BookOpen, end: false },
]

export default function App() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const toast = useToast()
  const [mobileOpen, setMobileOpen] = useState(false)
  const mobileButtonRef = useRef<HTMLButtonElement>(null)
  const mobileNavRef = useRef<HTMLElement>(null)
  const workspace =
    /^\/problems\/[^/]+$/.test(location.pathname) ||
    /^\/admin\/problems\/[^/]+\/package$/.test(location.pathname) ||
    /^\/contests\/[^/]+\/jury$/.test(location.pathname)

  useEffect(() => setMobileOpen(false), [location.pathname])

  useEffect(() => {
    let label: string | undefined
    if (location.pathname === '/login') label = '登录'
    else if (/^\/users\//.test(location.pathname)) label = '个人主页'
    else if (/^\/contests\/[^/]+\/jury$/.test(location.pathname)) label = '裁判台'
    else if (location.pathname.startsWith('/admin')) label = '管理后台'
    else {
      label = navigation.find((item) =>
        item.end ? location.pathname === item.to : location.pathname.startsWith(item.to),
      )?.label
    }
    document.title = label ? `${label} · Vertex` : 'Vertex Online Judge'
  }, [location.pathname])

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
    navigate('/')
  }

  return (
    <div className="flex min-h-screen flex-col bg-background">
      <a
        href="#main-content"
        className="fixed left-3 top-3 z-[100] -translate-y-20 border border-border bg-background px-3 py-2 text-sm focus:translate-y-0"
      >
        跳到主要内容
      </a>
      <header className="sticky top-0 z-40 border-b border-border bg-background/95 backdrop-blur-sm">
        <div className="mx-auto flex h-14 w-full max-w-[1440px] items-center gap-3 px-4 sm:px-6">
          <Link
            to="/"
            className="mr-3 text-sm font-semibold tracking-[0.14em] text-foreground transition-colors hover:text-primary"
          >
            <span>
              VERTEX<span className="text-primary">.</span>
            </span>
          </Link>

          <nav className="hidden h-full items-stretch gap-5 md:flex" aria-label="主导航">
            {navigation.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) =>
                  cn(
                    'relative inline-flex items-center px-0.5 text-sm transition-colors after:absolute after:inset-x-0 after:bottom-[-1px] after:h-0.5 after:origin-center after:scale-x-0 after:bg-primary after:transition-transform',
                    isActive
                      ? 'text-foreground after:scale-x-100'
                      : 'text-muted-foreground hover:text-foreground',
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

          <div className="ml-auto flex items-center gap-1.5">
            <ThemeToggle />
            {user ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="sm" className="gap-2">
                    <span className="grid size-6 place-items-center rounded-full bg-primary/15 text-xs font-semibold text-primary">
                      {user.username.slice(0, 1).toUpperCase()}
                    </span>
                    <span className="hidden max-w-28 truncate sm:inline">{user.username}</span>
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
                <Link to="/login">登录 / 注册</Link>
              </Button>
            )}
            <Button
              ref={mobileButtonRef}
              variant="ghost"
              size="icon"
              className="md:hidden"
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
            className="border-t border-border px-3 py-2 md:hidden"
            aria-label="移动端导航"
          >
            {navigation.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                onClick={() => setMobileOpen(false)}
                className={({ isActive }) =>
                  cn(
                    'flex min-h-11 items-center gap-2 border-l-2 px-3 py-2 text-sm',
                    isActive
                      ? 'border-primary text-foreground'
                      : 'border-transparent text-muted-foreground',
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
        <Suspense fallback={<RouteFallback />}>
          <Outlet />
        </Suspense>
      </main>

      {workspace ? null : (
        <footer className="border-t border-border py-5 text-center text-xs text-muted-foreground">
          Vertex Online Judge · 自托管在线判题平台
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
