import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { CalendarDays, ShieldCheck } from 'lucide-react'
import {
  getApiSubmissions as listSubmissions,
  getApiUsersUsername as getProfile,
} from '@/generated/api/vertex'
import type {
  DtoProfileResponse as Profile,
  DtoSubmissionResponse as Submission,
} from '@/generated/api/model'
import { DifficultyProgress } from '@/pages/HomePage'
import VerdictTag from '@/components/VerdictTag'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState, Skeleton, Tooltip } from '@/components/ui/misc'
import { formatDate, formatRatio, formatRelative } from '@/lib/format'
import { cn } from '@/lib/utils'

const ACTIVITY_DAYS = 91

export default function ProfilePage() {
  const { username } = useParams<{ username: string }>()
  const [profile, setProfile] = useState<Profile | null>(null)
  const [submissions, setSubmissions] = useState<Submission[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!username) return
    setLoading(true)
    Promise.allSettled([getProfile(username), listSubmissions({ user: username, size: 10 })])
      .then(([profileResult, submissionResult]) => {
        setProfile(profileResult.status === 'fulfilled' ? profileResult.value : null)
        if (submissionResult.status === 'fulfilled') setSubmissions(submissionResult.value.items)
      })
      .finally(() => setLoading(false))
  }, [username])

  if (loading) {
    return (
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-4 px-4 py-6">
        <Skeleton className="h-28 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  if (!profile) {
    return (
      <EmptyState
        title="用户不存在"
        description={`没有找到用户 ${username}。`}
        action={
          <Button variant="outline" asChild>
            <Link to="/">返回首页</Link>
          </Button>
        }
      />
    )
  }

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-4 px-4 py-6">
      <Card>
        <CardContent className="flex flex-wrap items-center gap-4 pt-5">
          <span className="grid size-14 shrink-0 place-items-center rounded-full bg-primary/15 text-xl font-semibold text-primary">
            {profile.username.slice(0, 1).toUpperCase()}
          </span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-xl font-semibold tracking-tight">{profile.username}</h1>
              {profile.role === 'admin' ? (
                <Badge variant="default">
                  <ShieldCheck />
                  管理员
                </Badge>
              ) : null}
            </div>
            <p className="mt-0.5 flex items-center gap-1.5 text-sm text-muted-foreground">
              <CalendarDays className="size-3.5" />
              {formatDate(profile.joinedAt)} 加入
            </p>
          </div>
          <div className="ml-auto grid grid-cols-3 gap-6 text-center">
            <Metric label="已通过" value={profile.solvedCount} />
            <Metric label="总提交" value={profile.submissionCount} />
            <Metric
              label="通过率"
              value={formatRatio(profile.acceptedCount, profile.submissionCount)}
            />
          </div>
        </CardContent>
      </Card>

      <ActivityHeatmap activity={profile.activity} />

      <div className="grid gap-4 lg:grid-cols-2">
        <DifficultyProgress profile={profile} />

        <Card>
          <CardHeader>
            <CardTitle>最近提交</CardTitle>
            <Link
              to={`/submissions?user=${profile.username}`}
              className="text-xs text-primary hover:underline"
            >
              查看全部
            </Link>
          </CardHeader>
          <CardContent>
            {submissions.length === 0 ? (
              <p className="py-4 text-sm text-muted-foreground">还没有提交记录。</p>
            ) : (
              <ul className="flex flex-col divide-y divide-border">
                {submissions.map((submission) => (
                  <li key={submission.id}>
                    <Link
                      to={`/submissions/${submission.id}`}
                      className="flex items-center gap-3 py-2.5 text-sm hover:text-primary"
                    >
                      <VerdictTag status={submission.status} />
                      <span className="min-w-0 flex-1 truncate">{submission.problemTitle}</span>
                      <span
                        className="shrink-0 text-xs text-muted-foreground"
                        title={submission.submittedAt}
                      >
                        {formatRelative(submission.submittedAt)}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function Metric({ label, value }: { label: string; value: number | string }) {
  return (
    <div>
      <p className="text-lg font-semibold tabular-nums">{value}</p>
      <p className="text-xs text-muted-foreground">{label}</p>
    </div>
  )
}

/**
 * Last ~13 weeks of submissions. The API only returns days that have activity,
 * so the empty days are filled in here to keep the grid continuous.
 */
function ActivityHeatmap({ activity }: { activity: Profile['activity'] }) {
  const days = useMemo(() => {
    const counts = new Map(activity.map((day) => [day.date, day.count]))
    const today = new Date()
    return Array.from({ length: ACTIVITY_DAYS }, (_, index) => {
      const date = new Date(today)
      date.setUTCDate(date.getUTCDate() - (ACTIVITY_DAYS - 1 - index))
      const key = date.toISOString().slice(0, 10)
      return { date: key, count: counts.get(key) ?? 0 }
    })
  }, [activity])

  const busiest = Math.max(1, ...days.map((day) => day.count))

  return (
    <Card>
      <CardHeader>
        <CardTitle>近 13 周活跃度</CardTitle>
        <span className="text-xs text-muted-foreground">
          共 {activity.reduce((sum, day) => sum + day.count, 0)} 次提交
        </span>
      </CardHeader>
      <CardContent>
        <div className="grid grid-flow-col grid-rows-7 gap-1 overflow-x-auto pb-1">
          {days.map((day) => (
            <Tooltip key={day.date} content={`${day.date} · ${day.count} 次提交`}>
              <span
                className={cn('size-3 rounded-[3px]', intensityClass(day.count, busiest))}
                aria-label={`${day.date} ${day.count} 次提交`}
              />
            </Tooltip>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

function intensityClass(count: number, busiest: number): string {
  if (count === 0) return 'bg-muted'
  const ratio = count / busiest
  if (ratio > 0.66) return 'bg-verdict-ac'
  if (ratio > 0.33) return 'bg-verdict-ac/70'
  return 'bg-verdict-ac/40'
}
