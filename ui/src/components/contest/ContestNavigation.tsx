import { useEffect, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { ArrowLeft, Clock3 } from 'lucide-react'
import { Link } from '@/domain/navigation'
import { useContestSpace } from './ContestContext'
import { cn } from '@/lib/utils'

export function ContestIdentity({
  drawer = false,
  onNavigate,
}: {
  drawer?: boolean
  onNavigate?: () => void
}) {
  const space = useContestSpace()
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])
  if (!space) return null
  const contest = space.details?.contest
  const started = contest && now >= Date.parse(contest.beginAt)
  const ended = contest && now >= Date.parse(contest.endAt)
  const seconds = contest
    ? Math.max(0, Math.ceil((Date.parse(started ? contest.endAt : contest.beginAt) - now) / 1000))
    : 0
  const time = `${Math.floor(seconds / 3600)
    .toString()
    .padStart(2, '0')}:${Math.floor((seconds / 60) % 60)
    .toString()
    .padStart(2, '0')}:${(seconds % 60).toString().padStart(2, '0')}`
  const role = space.details?.staffRole
  const label = contest?.permissions.viewJury
    ? contest.permissions.reply
      ? '裁判'
      : '观察员 · 只读'
    : contest?.permissions.edit
      ? '赛事编辑'
      : contest?.permissions.submit
        ? '选手'
        : '访客'
  return (
    <div
      className={cn(
        'flex min-w-0 gap-4',
        drawer
          ? 'flex-col border-b border-border pb-5'
          : 'items-center border-l border-border pl-4',
      )}
    >
      <div className={cn('flex min-w-0 flex-1 gap-3', drawer ? 'flex-col' : 'items-center')}>
        <Link
          to={space.workspaceHref}
          title={contest?.title ?? '比赛'}
          onClick={onNavigate}
          className={cn(
            'min-w-0 max-w-full font-semibold leading-6',
            drawer ? 'break-words text-lg' : 'truncate text-base',
          )}
        >
          {contest?.title ?? '比赛'}
        </Link>
        <div className="flex shrink-0 flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
          {contest && (
            <span className="rounded-md border border-border px-1.5 py-0.5 font-medium tracking-wide">
              {contest.format.toUpperCase()}
            </span>
          )}
          <span className="rounded-md bg-muted px-2 py-0.5">
            {role === 'observer' && !contest?.permissions.reply ? '观察员 · 只读' : label}
          </span>
        </div>
      </div>
      {contest && (
        <div className="flex shrink-0 items-center gap-2 whitespace-nowrap text-xs text-muted-foreground sm:text-sm">
          <Clock3 className="size-4 text-muted-foreground" />
          <span className={cn('tabular-nums', !ended && 'text-primary')}>
            {ended ? '已结束' : `${started ? '剩余' : '距开始'} ${time}`}
          </span>
        </div>
      )}
    </div>
  )
}

export function ContestNavigation({
  vertical = false,
  onNavigate,
}: {
  vertical?: boolean
  onNavigate?: () => void
}) {
  const space = useContestSpace()
  const { pathname, search } = useLocation()
  if (!space) return null
  const base = `/contests/${space.ref}`
  const tab =
    new URLSearchParams(search).get('tab') ?? (pathname.endsWith('/jury') ? 'board' : 'problems')
  const staff = space.details?.contest.permissions.viewJury
  const caps = space.details?.contest.permissions
  const inSubmissions =
    pathname.includes('/submissions') ||
    (pathname.endsWith('/jury') && ['submissions', 'rejudge'].includes(tab))
  const inManagement = ['settings', 'composition', 'access', 'staff'].includes(tab)
  const current = inSubmissions
    ? 'submissions'
    : inManagement
      ? 'management'
      : pathname.includes('/problems/')
        ? 'problems'
        : tab === 'board'
          ? 'rankboard'
          : tab
  const links = [
    { id: 'problems', label: '赛场', to: space.workspaceHref },
    { id: 'submissions', label: '提交记录', to: `${base}/submissions${staff ? '' : '?mine=1'}` },
    {
      id: 'rankboard',
      label: '榜单',
      to: staff ? `${base}/jury?tab=board` : `${base}?tab=rankboard`,
    },
    {
      id: 'clarifications',
      label: '公告与答疑',
      to: staff ? `${base}/jury?tab=clarifications` : `${base}?tab=clarifications`,
    },
    ...(caps?.edit || caps?.manageAccess
      ? [
          {
            id: 'management',
            label: '管理',
            to: `${base}?tab=${caps.edit ? 'settings' : 'access'}`,
          },
        ]
      : []),
  ]
  return (
    <nav
      aria-label="比赛导航"
      className={cn('flex gap-1', vertical ? 'flex-col' : 'site-container overflow-x-auto pb-2')}
    >
      <Link
        to="/contests"
        aria-label="返回域比赛列表"
        title="返回比赛列表"
        onClick={onNavigate}
        className={cn(
          'flex shrink-0 items-center text-muted-foreground hover:text-foreground',
          vertical
            ? 'mb-3 gap-2 rounded-lg px-3 py-3 text-sm hover:bg-muted'
            : 'mr-1 border-r border-border pr-3',
        )}
      >
        <ArrowLeft className="size-4" />
        {vertical && '返回比赛列表'}
      </Link>
      {links.map((item) => (
        <Link
          key={item.id}
          to={item.to}
          onClick={onNavigate}
          aria-current={
            (current === 'overview' ? 'problems' : current) === item.id ? 'page' : undefined
          }
          className={cn(
            'shrink-0 rounded-md px-3 py-2 text-sm transition-colors',
            vertical && 'flex min-h-11 items-center rounded-lg px-4',
            (current === 'overview' ? 'problems' : current) === item.id
              ? 'bg-primary/10 font-medium text-primary'
              : 'text-muted-foreground hover:bg-muted hover:text-foreground',
          )}
        >
          {item.label}
        </Link>
      ))}
    </nav>
  )
}
