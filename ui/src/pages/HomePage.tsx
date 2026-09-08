import { useEffect, useState } from 'react'
import { Link } from '@/domain/navigation'
import { ArrowRight, BookOpen, Code2, MessageSquare, Trophy } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type {
  DtoContestResponse as Contest,
  DtoProfileResponse as Profile,
  DtoSubmissionResponse as Submission,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import { problemHref } from '@/lib/routes'
import PageHeading from '@/components/PageHeading'
import HomeAnnouncements from '@/components/HomeAnnouncements'
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
  const {
    getApiContests: listContests,
    getApiSubmissions: listSubmissions,
    getApiUsersUsername: getProfile,
  } = useDomainAPI()
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
    <div className="page-shell">
      <PageHeading
        eyebrow={`你好，${username}`}
        title="今天，从一个好问题开始。"
        description="每一次思考，都在让下一道题变得更简单。"
        actions={
          <Button asChild>
            <Link to="/problems">
              开始练习 <ArrowRight />
            </Link>
          </Button>
        }
      />

      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_320px] xl:grid-cols-[minmax(0,1fr)_352px]">
        <div className="contents lg:flex lg:min-w-0 lg:flex-col lg:gap-6">
          <div className="order-1 grid gap-4 xl:grid-cols-2">
            <section className="surface-panel flex items-center justify-between gap-4 p-5 sm:p-6">
              <div className="min-w-0">
                <p className="mb-2 text-xs text-muted-foreground">
                  {submissions[0] ? '继续上次的练习' : '准备好开始了吗'}
                </p>
                <h2 className="truncate text-lg font-semibold">
                  {submissions[0]?.problemTitle || '挑一道题，进入状态'}
                </h2>
                <p className="mt-2 text-xs text-muted-foreground">
                  {submissions[0]
                    ? '代码草稿会自动保存，随时回来继续。'
                    : '从基础开始，按照自己的节奏慢慢进阶。'}
                </p>
              </div>
              <Button variant="secondary" asChild>
                <Link to={submissions[0] ? problemHref(submissions[0]) : '/problems'}>
                  继续 <ArrowRight />
                </Link>
              </Button>
            </section>
            <Link
              to="/problem-sets"
              className="surface-panel group flex items-center gap-4 p-5 transition-colors hover:border-primary/40 sm:p-6"
            >
              <span className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/7 text-primary">
                <BookOpen className="size-5" />
              </span>
              <div>
                <h2 className="text-sm font-semibold">循序渐进，找到练习路线</h2>
                <p className="mt-1.5 text-xs leading-5 text-muted-foreground">
                  把零散的知识，连成自己的算法地图。
                </p>
              </div>
              <ArrowRight className="ml-auto size-4 shrink-0 text-muted-foreground group-hover:text-primary" />
            </Link>
          </div>

          {hasLoadError ? (
            <div
              role="alert"
              className="order-3 flex flex-wrap items-center justify-between gap-3 border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm"
            >
              <span>部分个人概览暂时无法加载，未获取的数据不会显示为 0。</span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setReloadToken((value) => value + 1)}
              >
                重新加载
              </Button>
            </div>
          ) : null}

          <dl className="order-3 surface-panel grid grid-cols-2 divide-border sm:grid-cols-4 sm:divide-x">
            <StatCard label="已通过题目" value={profile?.solvedCount ?? '—'} />
            <StatCard label="尝试过题目" value={profile?.attemptedCount ?? '—'} />
            <StatCard label="总提交" value={profile?.submissionCount ?? '—'} />
            <StatCard
              label="提交通过率"
              value={profile ? formatRatio(profile.acceptedCount, profile.submissionCount) : '—'}
            />
          </dl>

          <section className="order-4 surface-panel overflow-hidden">
            <div className="flex items-center justify-between border-b border-border px-5 py-4">
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
                        to={`/submissions/${submission.publicId || submission.id}`}
                        className="flex items-center gap-3 px-5 py-4 text-sm transition-colors hover:bg-muted/50 hover:text-primary"
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
        </div>
        <aside className="contents lg:flex lg:min-w-0 lg:flex-col lg:gap-5">
          <HomeAnnouncements className="order-2" />

          <section className="order-5 surface-panel p-5">
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
                          to={`/contests/${contest.publicId || contest.id}`}
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

          {profile ? (
            <div className="order-6 surface-panel p-5">
              <DifficultyProgress profile={profile} />
            </div>
          ) : null}
        </aside>
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
    <div className="px-5 py-5 sm:px-6">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-2 text-[1.75rem] font-semibold leading-tight tracking-tight tabular-nums">
        {value}
      </dd>
    </div>
  )
}

function SectionError({ message }: { message: string }) {
  return <p className="py-4 text-sm text-destructive">{message}</p>
}

const features = [
  {
    icon: Code2,
    title: '专注练习',
    description: '挑一道题，写下思路。让清晰的评测反馈帮你走出下一步。',
  },
  {
    icon: Trophy,
    title: '参加一场比赛',
    description: '在有限的时间里尝试、调整，找到属于自己的解题节奏。',
  },
  {
    icon: BookOpen,
    title: '循序渐进',
    description: '用题单串起知识点，把零散的练习积累成完整的理解。',
  },
  {
    icon: MessageSquare,
    title: '题解与讨论',
    description: '分享一种解法，提出一个问题。好的思路值得一起讨论。',
  },
]

/** Anonymous home: what this is, and one obvious way in. */
function Landing() {
  return (
    <div className="page-shell flex flex-col gap-14">
      <section className="max-w-3xl py-10 sm:py-14">
        <p className="mb-4 text-xs font-medium uppercase tracking-[0.16em] text-primary">
          Vertex Online Judge
        </p>
        <h1 className="text-4xl font-semibold leading-tight tracking-tight sm:text-5xl">
          留一点时间，
          <br />
          <span className="text-muted-foreground">给值得思考的问题。</span>
        </h1>
        <p className="mt-5 max-w-xl text-base leading-7 text-muted-foreground">
          从第一道题到下一次突破。在这里练习算法、参加比赛，和认真思考的人交流。
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

      <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {features.map((feature) => (
          <div key={feature.title} className="surface-panel p-6">
            <feature.icon className="size-4 text-primary" />
            <h2 className="mt-3 text-sm font-semibold">{feature.title}</h2>
            <p className="mt-1 text-sm text-muted-foreground">{feature.description}</p>
          </div>
        ))}
      </section>
    </div>
  )
}
