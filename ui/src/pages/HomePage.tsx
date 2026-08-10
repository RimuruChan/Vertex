import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, Code2, MessageSquare, ShieldCheck, Trophy } from 'lucide-react'
import {
  getApiContests as listContests,
  getApiSubmissions as listSubmissions,
  getApiUsersUsername as getProfile,
} from '@/generated/api/vertex'
import type {
  DtoContestResponse as Contest,
  DtoProfileResponse as Profile,
  DtoSubmissionResponse as Submission,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import VerdictTag from '@/components/VerdictTag'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState, Progress, Skeleton } from '@/components/ui/misc'
import { formatRelative, formatRatio, shortId } from '@/lib/format'

export default function HomePage() {
  const { user, ready } = useAuth()
  if (!ready) return <Skeleton className="m-6 h-64" />
  return user ? <Dashboard username={user.username} /> : <Landing />
}

/** Signed-in home: your progress and what needs your attention, not marketing. */
function Dashboard({ username }: { username: string }) {
  const [profile, setProfile] = useState<Profile | null>(null)
  const [submissions, setSubmissions] = useState<Submission[]>([])
  const [contests, setContests] = useState<Contest[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    // Three independent reads; a slow one must not block the others.
    Promise.allSettled([
      getProfile(username),
      listSubmissions({ user: username, size: 8 }),
      listContests({ size: 5 }),
    ])
      .then(([profileResult, submissionResult, contestResult]) => {
        if (profileResult.status === 'fulfilled') setProfile(profileResult.value)
        if (submissionResult.status === 'fulfilled') setSubmissions(submissionResult.value.items)
        if (contestResult.status === 'fulfilled') setContests(contestResult.value.items)
      })
      .finally(() => setLoading(false))
  }, [username])

  if (loading) {
    return (
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-6">
        <Skeleton className="h-28 w-full" />
        <Skeleton className="h-72 w-full" />
      </div>
    )
  }

  const now = Date.now()
  const upcoming = contests
    .filter((contest) => new Date(contest.endAt).getTime() > now)
    .sort((a, b) => new Date(a.beginAt).getTime() - new Date(b.beginAt).getTime())

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">你好,{username}</h1>
        <p className="text-sm text-muted-foreground">继续昨天的进度,或者挑一道新题。</p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label="已通过题目" value={profile?.solvedCount ?? 0} />
        <StatCard label="尝试过题目" value={profile?.attemptedCount ?? 0} />
        <StatCard label="总提交" value={profile?.submissionCount ?? 0} />
        <StatCard
          label="提交通过率"
          value={formatRatio(profile?.acceptedCount ?? 0, profile?.submissionCount ?? 0)}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>最近提交</CardTitle>
            <Link to="/submissions?mine=1" className="text-xs text-primary hover:underline">
              查看全部
            </Link>
          </CardHeader>
          <CardContent>
            {submissions.length === 0 ? (
              <EmptyState
                icon={<Code2 />}
                title="还没有提交"
                description="从一道入门题开始。"
                action={
                  <Button asChild>
                    <Link to="/problems">前往题库</Link>
                  </Button>
                }
              />
            ) : (
              <ul className="flex flex-col divide-y divide-border">
                {submissions.map((submission) => (
                  <li key={submission.id}>
                    <Link
                      to={`/submissions/${submission.id}`}
                      className="flex items-center gap-3 py-2.5 text-sm hover:text-primary"
                    >
                      <VerdictTag status={submission.status} />
                      <span className="min-w-0 flex-1 truncate font-medium">
                        {submission.problemTitle}
                      </span>
                      <span className="hidden font-mono text-xs text-muted-foreground sm:inline">
                        #{shortId(submission.id)}
                      </span>
                      <span
                        className="w-20 shrink-0 text-right text-xs text-muted-foreground"
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

        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>比赛</CardTitle>
              <Link to="/contests" className="text-xs text-primary hover:underline">
                全部
              </Link>
            </CardHeader>
            <CardContent>
              {upcoming.length === 0 ? (
                <p className="py-4 text-sm text-muted-foreground">暂时没有进行中或即将开始的比赛。</p>
              ) : (
                <ul className="flex flex-col divide-y divide-border">
                  {upcoming.slice(0, 4).map((contest) => {
                    const running = new Date(contest.beginAt).getTime() <= now
                    return (
                      <li key={contest.id}>
                        <Link
                          to={`/contests/${contest.id}`}
                          className="flex items-center gap-2 py-2.5 text-sm hover:text-primary"
                        >
                          <span
                            className={
                              running
                                ? 'size-1.5 shrink-0 animate-pulse rounded-full bg-verdict-ac'
                                : 'size-1.5 shrink-0 rounded-full bg-muted-foreground/50'
                            }
                          />
                          <span className="min-w-0 flex-1 truncate">{contest.title}</span>
                          <span className="shrink-0 text-xs text-muted-foreground">
                            {running ? '进行中' : formatRelative(contest.beginAt)}
                          </span>
                        </Link>
                      </li>
                    )
                  })}
                </ul>
              )}
            </CardContent>
          </Card>

          {profile ? <DifficultyProgress profile={profile} /> : null}
        </div>
      </div>
    </div>
  )
}

export function DifficultyProgress({ profile }: { profile: Profile }) {
  const buckets = profile.byDifficulty.filter((bucket) => bucket.total > 0)
  if (buckets.length === 0) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle>难度分布</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2.5">
        {buckets.map((bucket) => (
          <div key={bucket.difficulty} className="flex items-center gap-3 text-xs">
            <span className="w-14 shrink-0 whitespace-nowrap text-muted-foreground">
              难度 {bucket.difficulty}
            </span>
            <Progress
              className="flex-1"
              value={bucket.total > 0 ? (bucket.solved / bucket.total) * 100 : 0}
            />
            <span className="w-14 shrink-0 text-right tabular-nums text-muted-foreground">
              {bucket.solved} / {bucket.total}
            </span>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}

function StatCard({ label, value }: { label: string; value: number | string }) {
  return (
    <Card className="px-5 py-4">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
    </Card>
  )
}

const features = [
  {
    icon: Code2,
    title: '在线判题',
    description: 'C / C++ / Python,逐测试点反馈,实时评测进度。',
  },
  {
    icon: Trophy,
    title: 'ACM/ICPC 比赛',
    description: '报名、实时榜单、封榜与解榜,赛后转为练习。',
  },
  {
    icon: ShieldCheck,
    title: '隔离沙箱',
    description: 'Landlock、seccomp、cgroup v2 约束不受信任的代码。',
  },
  {
    icon: MessageSquare,
    title: '题解与讨论',
    description: 'Markdown + LaTeX 渲染,线程式回复。',
  },
]

/** Anonymous home: what this is, and one obvious way in. */
function Landing() {
  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-10 px-4 py-14">
      <section className="flex flex-col items-center gap-5 text-center">
        <h1 className="max-w-2xl text-3xl font-semibold tracking-tight sm:text-4xl">
          自托管的在线判题平台
        </h1>
        <p className="max-w-xl text-muted-foreground">
          题目、比赛、题解与讨论,跑在你自己的机器上。
        </p>
        <div className="flex flex-wrap justify-center gap-2">
          <Button size="lg" asChild>
            <Link to="/problems">
              浏览题库
              <ArrowRight />
            </Link>
          </Button>
          <Button size="lg" variant="outline" asChild>
            <Link to="/login">登录 / 注册</Link>
          </Button>
        </div>
      </section>

      <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {features.map((feature) => (
          <Card key={feature.title} className="px-5 py-5">
            <feature.icon className="size-5 text-primary" />
            <h2 className="mt-3 text-sm font-semibold">{feature.title}</h2>
            <p className="mt-1 text-sm text-muted-foreground">{feature.description}</p>
          </Card>
        ))}
      </section>
    </div>
  )
}
