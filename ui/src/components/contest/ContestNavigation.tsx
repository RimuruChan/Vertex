import { useEffect, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { ArrowLeft, Clock3 } from 'lucide-react'
import { Link } from '@/domain/navigation'
import { useDomain } from '@/domain/DomainContext'
import { useContestSpace } from './ContestContext'
import { cn } from '@/lib/utils'
import VertexLogo from '@/components/VertexLogo'

export function ContestIdentity() {
  const space = useContestSpace()
  const { domain } = useDomain()
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
    <div className="flex min-w-0 flex-1 items-center gap-3 sm:gap-5">
      <Link
        to="/contests"
        className="flex shrink-0 items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        aria-label="返回域比赛列表"
      >
        <ArrowLeft className="size-4" />
        <VertexLogo className="[&>span]:hidden sm:[&>span]:inline" />
        {!domain.official && <span className="hidden sm:inline">{domain.name}</span>}
      </Link>
      <div className="min-w-0 flex-1">
        <Link
          to={space.workspaceHref}
          className="block truncate text-sm font-semibold sm:text-base"
        >
          {contest?.title ?? '比赛'}
        </Link>
        <div className="mt-0.5 flex items-center gap-2 text-xs text-muted-foreground">
          <span>{contest?.format.toUpperCase()}</span>
          <span>
            {role === 'observer' && !contest?.permissions.reply ? '观察员 · 只读' : label}
          </span>
        </div>
      </div>
      {contest && (
        <div className="flex shrink-0 items-center gap-2 text-xs sm:text-sm">
          <Clock3 className="hidden size-4 text-muted-foreground sm:block" />
          <span className={cn('tabular-nums', !ended && 'text-primary')}>
            {ended ? '已结束' : `${started ? '剩余' : '距开始'} ${time}`}
          </span>
        </div>
      )}
    </div>
  )
}

export function ContestNavigation() {
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
      className="mx-auto flex w-full max-w-[1440px] gap-1 overflow-x-auto px-3 pb-2 sm:px-6"
    >
      {links.map((item) => (
        <Link
          key={item.id}
          to={item.to}
          aria-current={
            (current === 'overview' ? 'problems' : current) === item.id ? 'page' : undefined
          }
          className={cn(
            'shrink-0 rounded-md px-3 py-2 text-sm transition-colors',
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
