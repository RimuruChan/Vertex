import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, Code2, Megaphone, MessageSquare, Pin, ShieldCheck, Trophy } from 'lucide-react'
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
import { getApiAnnouncements as listAnnouncements } from '@/generated/api/vertex'
import type { DtoAnnouncementResponse as Announcement } from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import MdRenderer from '@/components/MdRenderer'
import VerdictTag from '@/components/VerdictTag'
import { Button } from '@/components/ui/button'
import { EmptyState, Progress, Skeleton } from '@/components/ui/misc'
import { apiError, formatRelative, formatRatio, shortId } from '@/lib/format'

export default function HomePage() {
  const { user, ready } = useAuth()
  if (!ready) return <Skeleton className="m-6 h-64" />
  return user ? <Dashboard username={user.username} /> : <Landing />
}

/** Signed-in home: your progress and what needs your attention, not marketing. */
function Dashboard({ username }: { username: string }) {
  const [announcements, setAnnouncements] = useState<Announcement[]>([])

  useEffect(() => {
    listAnnouncements({ limit: 3 })
      .then((result) => setAnnouncements(result.items))
      .catch(() => setAnnouncements([]))
  }, [])
  const [profile, setProfile] = useState<Profile | null>(null)
  const [submissions, setSubmissions] = useState<Submission[]>([])
  const [contests, setContests] = useState<Contest[]>([])
  const [loading, setLoading] = useState(true)
  const [loadErrors, setLoadErrors] = useState<{
    profile?: string
    submissions?: string
    contests?: string
  }>({})
  const [reloadToken, setReloadToken] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setLoadErrors({})
    setProfile(null)
    setSubmissions([])
    setContests([])
    Promise.allSettled([
      getProfile(username, { signal: controller.signal }),
      listSubmissions({ user: username, size: 8 }, { signal: controller.signal }),
      listContests({ size: 5 }, { signal: controller.signal }),
    ])
      .then(([profileResult, submissionResult, contestResult]) => {
        if (controller.signal.aborted) return
        if (profileResult.status === 'fulfilled') setProfile(profileResult.value)
        if (submissionResult.status === 'fulfilled') setSubmissions(submissionResult.value.items)
        if (contestResult.status === 'fulfilled') setContests(contestResult.value.items)
        setLoadErrors({
          profile:
            profileResult.status === 'rejected'
              ? apiError(profileResult.reason, '个人统计加载失败')
              : undefined,
          submissions:
            submissionResult.status === 'rejected'
              ? apiError(submissionResult.reason, '最近提交加载失败')
              : undefined,
          contests:
            contestResult.status === 'rejected'
              ? apiError(contestResult.reason, '比赛列表加载失败')
              : undefined,
        })
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [reloadToken, username])

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
  const hasLoadError = Boolean(loadErrors.profile || loadErrors.submissions || loadErrors.contests)

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-8 px-4 py-8 sm:px-6 sm:py-10">
      <header className="border-b border-border pb-6">
        <p className="mb-2 text-xs text-muted-foreground">个人概览</p>
        <h1 className="text-2xl font-semibold tracking-tight">你好，{username}</h1>
        <p className="mt-2 text-sm text-muted-foreground">继续上次的进度，或者挑一道新题。</p>
      </header>

      {announcements.length > 0 ? (
        <section className="divide-y divide-border border-y border-border" aria-label="站点公告">
          {announcements.map((notice) => (
            <article key={notice.id} className="py-4">
              <div className="flex items-center gap-2 text-sm">
                {notice.pinned ? (
                  <Pin className="size-4 text-primary" />
                ) : (
                  <Megaphone className="size-4 text-muted-foreground" />
                )}
                <span className="font-medium">{notice.title}</span>
                <span className="ml-auto text-xs text-muted-foreground">
                  {formatRelative(notice.createdAt)}
                </span>
              </div>
              {notice.contentMd ? (
                <MdRenderer content={notice.contentMd} className="mt-2 text-sm" />
              ) : null}
            </article>
          ))}
        </section>
      ) : null}

      {hasLoadError ? (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm"
        >
          <span>部分个人概览暂时无法加载，未获取的数据不会显示为 0。</span>
          <Button variant="outline" size="sm" onClick={() => setReloadToken((value) => value + 1)}>
            重新加载
          </Button>
        </div>
      ) : null}

      <dl className="grid grid-cols-2 border-y border-border sm:grid-cols-4 sm:divide-x sm:divide-border">
        <StatCard label="已通过题目" value={profile?.solvedCount ?? '—'} />
        <StatCard label="尝试过题目" value={profile?.attemptedCount ?? '—'} />
        <StatCard label="总提交" value={profile?.submissionCount ?? '—'} />
        <StatCard
          label="提交通过率"
          value={profile ? formatRatio(profile.acceptedCount, profile.submissionCount) : '—'}
        />
      </dl>

      <div className="grid gap-10 lg:grid-cols-3">
        <section className="lg:col-span-2">
          <div className="flex items-center justify-between border-b border-border pb-3">
            <h2 className="text-sm font-semibold">最近提交</h2>
            <Link to="/submissions?mine=1" className="text-xs text-primary hover:underline">
              查看全部
            </Link>
          </div>
          <div>
            {loadErrors.submissions ? (
              <SectionError message={loadErrors.submissions} />
            ) : submissions.length === 0 ? (
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
          </div>
        </section>

        <div className="flex flex-col gap-8">
          <section>
            <div className="flex items-center justify-between border-b border-border pb-3">
              <h2 className="text-sm font-semibold">比赛</h2>
              <Link to="/contests" className="text-xs text-primary hover:underline">
                全部
              </Link>
            </div>
            <div>
              {loadErrors.contests ? (
                <SectionError message={loadErrors.contests} />
              ) : upcoming.length === 0 ? (
                <p className="py-4 text-sm text-muted-foreground">
                  暂时没有进行中或即将开始的比赛。
                </p>
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
            </div>
          </section>

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
    <section>
      <h2 className="border-b border-border pb-3 text-sm font-semibold">难度分布</h2>
      <div className="mt-4 flex flex-col gap-2.5">
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
      </div>
    </section>
  )
}

function StatCard({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="px-3 py-4 first:pl-0 sm:px-5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 text-2xl font-semibold tabular-nums">{value}</dd>
    </div>
  )
}

function SectionError({ message }: { message: string }) {
  return <p className="py-4 text-sm text-destructive">{message}</p>
}

const features = [
  {
    icon: Code2,
    title: '在线判题',
    description: 'C / C++ / Python，逐测试点反馈，实时评测进度。',
  },
  {
    icon: Trophy,
    title: 'ACM/ICPC 比赛',
    description: '报名、实时榜单、封榜与解榜，赛后转为练习。',
  },
  {
    icon: ShieldCheck,
    title: '隔离沙箱',
    description: 'Landlock、seccomp、cgroup v2 约束不受信任的代码。',
  },
  {
    icon: MessageSquare,
    title: '题解与讨论',
    description: 'Markdown + LaTeX 渲染，线程式回复。',
  },
]

/** Anonymous home: what this is, and one obvious way in. */
function Landing() {
  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-14 px-4 py-16 sm:px-6 sm:py-24">
      <section className="max-w-2xl">
        <p className="mb-4 text-xs font-medium uppercase tracking-[0.16em] text-primary">
          Vertex Online Judge
        </p>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-5xl">自托管的在线判题平台</h1>
        <p className="mt-5 max-w-xl text-base leading-7 text-muted-foreground">
          题目、比赛、题解与讨论，运行在你自己的机器上。
        </p>
        <div className="mt-7 flex flex-wrap gap-2">
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

      <section className="grid border-y border-border sm:grid-cols-2">
        {features.map((feature) => (
          <div
            key={feature.title}
            className="border-b border-border py-6 last:border-b-0 sm:px-5 sm:[&:nth-child(odd)]:border-r sm:[&:nth-last-child(-n+2)]:border-b-0"
          >
            <feature.icon className="size-4 text-primary" />
            <h2 className="mt-3 text-sm font-semibold">{feature.title}</h2>
            <p className="mt-1 text-sm text-muted-foreground">{feature.description}</p>
          </div>
        ))}
      </section>
    </div>
  )
}
