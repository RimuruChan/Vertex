import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { CalendarClock, Lock, Trophy } from 'lucide-react'
import { getApiContests as listContests } from '@/generated/api/vertex'
import type { DtoContestResponse as Contest } from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import PageHeading from '@/components/PageHeading'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import { Pagination } from '@/components/ui/pagination'
import { apiError, formatDateTime } from '@/lib/format'

const PAGE_SIZE = 20

type Phase = { label: string; variant: 'default' | 'secondary' | 'success' | 'outline' }

export function contestPhase(contest: Contest, now = Date.now()): Phase {
  if (now < new Date(contest.beginAt).getTime()) return { label: '未开始', variant: 'secondary' }
  if (now <= new Date(contest.endAt).getTime()) return { label: '进行中', variant: 'success' }
  return { label: '已结束', variant: 'outline' }
}

function positivePage(value: string | null) {
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : 1
}

function durationHours(contest: Contest): string {
  const ms = new Date(contest.endAt).getTime() - new Date(contest.beginAt).getTime()
  const hours = ms / 3_600_000
  return hours >= 1 ? `${Number(hours.toFixed(1))} 小时` : `${Math.round(ms / 60_000)} 分钟`
}

export default function ContestListPage() {
  const { user, ready } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()
  const page = positivePage(searchParams.get('page'))

  const [contests, setContests] = useState<Contest[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadToken, setReloadToken] = useState(0)
  const [clock, setClock] = useState(Date.now())

  useEffect(() => {
    const timer = window.setInterval(() => setClock(Date.now()), 15_000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    if (!ready) return
    const controller = new AbortController()
    setLoading(true)
    setLoadError(null)
    listContests({ page, size: PAGE_SIZE }, { signal: controller.signal })
      .then((result) => {
        if (controller.signal.aborted) return
        setContests(result.items)
        setTotal(result.total)
      })
      .catch((error) => {
        if (controller.signal.aborted) return
        setContests([])
        setTotal(0)
        setLoadError(apiError(error, '比赛列表加载失败'))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [page, ready, reloadToken, user?.id])

  return (
    <div className="page-shell">
      <PageHeading
        eyebrow="挑战 / 比赛"
        title="在挑战中，找到自己的节奏。"
        description={
          loading
            ? '正在加载比赛…'
            : loadError
              ? '比赛总数暂不可用'
              : `${total} 场比赛 · 赛时专注解题，赛后一起复盘。`
        }
      />

      {loading ? (
        <div className="flex flex-col gap-3">
          {Array.from({ length: 4 }, (_, index) => (
            <Skeleton key={index} className="h-24 w-full" />
          ))}
        </div>
      ) : loadError ? (
        <Card>
          <EmptyState
            icon={<Trophy />}
            title="比赛列表加载失败"
            description={loadError}
            action={
              <Button variant="outline" onClick={() => setReloadToken((value) => value + 1)}>
                重试
              </Button>
            }
          />
        </Card>
      ) : contests.length === 0 ? (
        <Card>
          <EmptyState
            icon={<Trophy />}
            title="暂时没有比赛"
            description="等待管理员创建下一场比赛。"
          />
        </Card>
      ) : (
        <>
          <div className="mb-5 flex flex-col gap-4">
            {contests.map((contest) => {
              const phase = contestPhase(contest, clock)
              return (
                <Link key={contest.id} to={`/contests/${contest.id}`}>
                  <Card className="px-6 py-6 transition-colors hover:border-primary/40">
                    <div className="flex flex-wrap items-center gap-2">
                      <Trophy className="mr-2 size-5 text-primary" />
                      <h2 className="mr-2 text-lg font-semibold">{contest.title}</h2>
                      <Badge variant={phase.variant}>{phase.label}</Badge>
                      <Badge variant="outline">{contest.rule.toUpperCase()}</Badge>
                      {contest.visibility === 'password' ? (
                        <Badge variant="outline">
                          <Lock />
                          需要密码
                        </Badge>
                      ) : null}
                    </div>
                    {contest.description ? (
                      <p className="mt-3 line-clamp-2 max-w-3xl text-sm leading-6 text-muted-foreground">
                        {contest.description}
                      </p>
                    ) : null}
                    <p className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
                      <span className="inline-flex items-center gap-1.5">
                        <CalendarClock className="size-3.5" />
                        {formatDateTime(contest.beginAt)}
                      </span>
                      <span>时长 {durationHours(contest)}</span>
                    </p>
                  </Card>
                </Link>
              )
            })}
          </div>
          <Card className="overflow-hidden">
            <Pagination
              page={page}
              size={PAGE_SIZE}
              total={total}
              onChange={(next) => setSearchParams(next > 1 ? { page: String(next) } : {})}
            />
          </Card>
        </>
      )}
    </div>
  )
}
