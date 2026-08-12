import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { CalendarClock, Lock, Trophy } from 'lucide-react'
import { getApiContests as listContests } from '@/generated/api/vertex'
import type { DtoContestResponse as Contest } from '@/generated/api/model'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import { Pagination } from '@/components/ui/pagination'
import { useToast } from '@/components/ui/toast'
import { apiError, formatDateTime } from '@/lib/format'

const PAGE_SIZE = 20

type Phase = { label: string; variant: 'default' | 'secondary' | 'success' | 'outline' }

export function contestPhase(contest: Contest): Phase {
  const now = Date.now()
  if (now < new Date(contest.beginAt).getTime()) return { label: '未开始', variant: 'secondary' }
  if (now <= new Date(contest.endAt).getTime()) return { label: '进行中', variant: 'success' }
  return { label: '已结束', variant: 'outline' }
}

function durationHours(contest: Contest): string {
  const ms = new Date(contest.endAt).getTime() - new Date(contest.beginAt).getTime()
  const hours = ms / 3_600_000
  return hours >= 1 ? `${Number(hours.toFixed(1))} 小时` : `${Math.round(ms / 60_000)} 分钟`
}

export default function ContestListPage() {
  const toast = useToast()
  const [searchParams, setSearchParams] = useSearchParams()
  const page = Math.max(1, Number(searchParams.get('page') ?? 1))

  const [contests, setContests] = useState<Contest[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    listContests({ page, size: PAGE_SIZE })
      .then((result) => {
        setContests(result.items)
        setTotal(result.total)
      })
      .catch((error) => toast.error(apiError(error, '比赛列表加载失败')))
      .finally(() => setLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page])

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-4 px-4 py-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">比赛</h1>
        <p className="text-sm text-muted-foreground">共 {total} 场</p>
      </div>

      {loading ? (
        <div className="flex flex-col gap-3">
          {Array.from({ length: 4 }, (_, index) => (
            <Skeleton key={index} className="h-24 w-full" />
          ))}
        </div>
      ) : contests.length === 0 ? (
        <Card>
          <EmptyState icon={<Trophy />} title="暂时没有比赛" description="等待管理员创建下一场比赛。" />
        </Card>
      ) : (
        <>
          <div className="flex flex-col gap-3">
            {contests.map((contest) => {
              const phase = contestPhase(contest)
              return (
                <Link key={contest.id} to={`/contests/${contest.id}`}>
                  <Card className="px-5 py-4 transition-colors hover:border-primary/60">
                    <div className="flex flex-wrap items-center gap-2">
                      <h2 className="text-base font-semibold">{contest.title}</h2>
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
                      <p className="mt-1.5 line-clamp-2 text-sm text-muted-foreground">
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
