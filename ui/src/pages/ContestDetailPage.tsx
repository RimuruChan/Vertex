import { useEffect, useRef, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { Link, useNavigate } from '@/domain/navigation'
import {
  CalendarClock,
  EyeOff,
  Gavel,
  ListChecks,
  Lock,
  MessagesSquare,
  Snowflake,
  Trophy,
} from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type {
  DtoContestProblemResponse as ContestProblem,
  DtoContestResponse as Contest,
  DtoRankboardResponse as Rankboard,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import { useCanonicalResourcePath } from '@/hooks/useCanonicalPath'
import { problemHref } from '@/lib/routes'
import Clarifications from '@/components/contest/Clarifications'
import Scoreboard from '@/components/contest/Scoreboard'
import { contestPhase } from '@/pages/ContestListPage'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import ResourceCollaboration from '@/components/ResourceCollaboration'
import ContestSettings from '@/components/contest/ContestSettings'
import ContestComposition from '@/components/contest/ContestComposition'
import { canPrepareContest } from '@/components/contest/contest-form'
import { useToast } from '@/components/ui/toast'
import { apiError, formatDateTime } from '@/lib/format'

const RANKBOARD_POLL_MS = 5000

type BoardStatus = 'idle' | 'loading' | 'ready' | 'error'

type BoardState = {
  status: BoardStatus
  data: Rankboard | null
  error: string | null
}

const emptyBoardState: BoardState = { status: 'idle', data: null, error: null }

/** Contest problems are labelled A, B, C… everywhere they appear. */
function problemLetter(index: number): string {
  return String.fromCharCode(65 + index)
}

export default function ContestDetailPage() {
  const {
    getApiContestsId: getContest,
    getApiContestsIdRankboard: getContestRankboard,
    getApiContestsIdRegistration: getContestRegistration,
    postApiContestsIdRegister: registerContest,
  } = useDomainAPI()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const toast = useToast()
  const { user, ready } = useAuth()

  const [contest, setContest] = useState<Contest | null>(null)
  useCanonicalResourcePath('contests', id, contest)
  const [problems, setProblems] = useState<ContestProblem[]>([])
  const [boardState, setBoardState] = useState<BoardState>(emptyBoardState)
  const [registered, setRegistered] = useState(false)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [notFound, setNotFound] = useState(false)
  const [registrationError, setRegistrationError] = useState<string | null>(null)
  const [reloadToken, setReloadToken] = useState(0)
  const [boardReloadToken, setBoardReloadToken] = useState(0)
  const [params, setParams] = useSearchParams()
  const activeTab = params.get('tab') ?? 'problems'
  function setActiveTab(value: string) {
    setParams(
      (previous) => {
        const next = new URLSearchParams(previous)
        if (value === 'problems') next.delete('tab')
        else next.set('tab', value)
        return next
      },
      { replace: true },
    )
  }
  const refreshRequest = useRef<AbortController | null>(null)
  const [refreshError, setRefreshError] = useState<string | null>(null)
  const [clock, setClock] = useState(() => Date.now())
  const [loadedContext, setLoadedContext] = useState('')
  const [registering, setRegistering] = useState(false)
  const [passwordOpen, setPasswordOpen] = useState(false)
  const [password, setPassword] = useState('')

  const contextKey = `${id ?? ''}:${user?.id ?? 'anonymous'}:${user?.role ?? ''}`
  const isStaff = Boolean(contest?.permissions.viewJury)
  const canSeeClarifications =
    Boolean(user) && (isStaff || registered || contest?.visibility === 'public')
  const started = contest !== null && clock >= new Date(contest.beginAt).getTime()
  const ended = contest !== null && clock > new Date(contest.endAt).getTime()
  const contestRunning = started && !ended
  const canRequestBoard =
    contest !== null &&
    (isStaff || (contest.rankboardVisible && (contest.visibility !== 'password' || registered)))

  useEffect(() => () => refreshRequest.current?.abort(), [])

  // Refresh capabilities and composition without unmounting either editing draft.
  async function refreshDetails() {
    if (!id) return
    refreshRequest.current?.abort()
    const controller = new AbortController()
    refreshRequest.current = controller
    setRefreshError(null)
    try {
      const details = await getContest(id, { signal: controller.signal })
      if (controller.signal.aborted) return
      setContest(details.contest)
      setProblems(details.problems)
      setBoardReloadToken((value) => value + 1)
    } catch (error) {
      if (controller.signal.aborted) return
      const status = (error as { response?: { status?: number } })?.response?.status
      if (status === 401 || status === 403 || status === 404) {
        setContest(null)
        setNotFound(true)
      } else setRefreshError(apiError(error, '操作已完成，但详情刷新失败；请重试核对最新状态。'))
    }
  }

  useEffect(() => {
    if (!ready) return
    if (!id) {
      setContest(null)
      setNotFound(true)
      setLoading(false)
      return
    }
    const controller = new AbortController()
    setLoading(true)
    setLoadError(null)
    setNotFound(false)
    setRegistrationError(null)
    setRegistered(false)
    setProblems([])
    setContest(null)
    setBoardState(emptyBoardState)
    setPasswordOpen(false)
    setPassword('')

    const registrationRequest: Promise<{ registered: boolean; error: string | null }> = user
      ? getContestRegistration(id, { signal: controller.signal }).then(
          (result) => ({ registered: result.registered, error: null }),
          (error) => ({
            registered: false,
            error: controller.signal.aborted ? null : apiError(error, '报名状态加载失败，请重试'),
          }),
        )
      : Promise.resolve({ registered: false, error: null })

    Promise.all([getContest(id, { signal: controller.signal }), registrationRequest])
      .then(([details, registration]) => {
        if (controller.signal.aborted) return
        setContest(details.contest)
        setProblems(details.problems)
        setRegistered(registration.registered)
        setRegistrationError(registration.error)
        setClock(Date.now())
        setLoadedContext(contextKey)
      })
      .catch((error) => {
        if (controller.signal.aborted) return
        setContest(null)
        if ((error as { response?: { status?: number } })?.response?.status === 404) {
          setNotFound(true)
          return
        }
        setLoadError(apiError(error, '比赛加载失败，请稍后重试'))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })

    return () => controller.abort()
  }, [contextKey, id, ready, reloadToken, user])

  // Keep phase-dependent controls accurate without polling the rankboard before
  // the contest begins or after it ends.
  useEffect(() => {
    if (!contest || ended) return
    const timer = window.setInterval(() => setClock(Date.now()), 15_000)
    return () => window.clearInterval(timer)
  }, [contest, ended])

  useEffect(() => {
    if (activeTab !== 'rankboard' || !id || !canRequestBoard) return

    const controller = new AbortController()
    let stopped = false
    let timer: number | undefined

    const refresh = async (initial: boolean) => {
      if (initial) {
        setBoardState((current) => ({ status: 'loading', data: current.data, error: null }))
      }
      try {
        const result = await getContestRankboard(id, {}, { signal: controller.signal })
        if (!controller.signal.aborted) {
          setBoardState({ status: 'ready', data: result, error: null })
        }
      } catch (error) {
        if (controller.signal.aborted) return
        const status = (error as { response?: { status?: number } })?.response?.status
        setBoardState((current) => ({
          status: 'error',
          data: status === 401 || status === 403 ? null : current.data,
          error: apiError(error, '榜单加载失败，请稍后重试'),
        }))
      }
    }

    const poll = async () => {
      if (!document.hidden) await refresh(false)
      if (!stopped) timer = window.setTimeout(poll, RANKBOARD_POLL_MS)
    }

    void refresh(true).then(() => {
      if (!stopped && contestRunning) timer = window.setTimeout(poll, RANKBOARD_POLL_MS)
    })

    return () => {
      stopped = true
      controller.abort()
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [activeTab, boardReloadToken, canRequestBoard, contestRunning, id])

  useEffect(() => {
    if (loading || !contest) return
    if (
      !['problems', 'rankboard', 'clarifications', 'settings', 'composition', 'access'].includes(
        activeTab,
      ) ||
      (activeTab === 'clarifications' && !canSeeClarifications) ||
      (['settings', 'composition', 'access'].includes(activeTab) &&
        !contest.permissions.previewProblems)
    )
      setActiveTab('problems')
  }, [activeTab, canSeeClarifications, contest?.permissions.previewProblems, loading])

  async function handleRegister(contestPassword?: string) {
    if (!id) return
    if (!user) {
      toast.warning('请先登录')
      navigate('/login', { state: { from: `/contests/${id}` } })
      return
    }
    if (contest?.visibility === 'password' && contestPassword === undefined) {
      setPasswordOpen(true)
      return
    }
    setRegistering(true)
    try {
      await registerContest(id, contestPassword ? { password: contestPassword } : {})
      toast.success('报名成功')
      setPasswordOpen(false)
      setPassword('')
      setReloadToken((value) => value + 1)
    } catch (error) {
      toast.error(apiError(error, '报名失败'))
    } finally {
      setRegistering(false)
    }
  }

  const board = boardState.data

  if (!ready || loading || (contest !== null && loadedContext !== contextKey)) {
    return (
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 px-4 py-6">
        <Skeleton className="h-28 w-full" />
        <Skeleton className="h-72 w-full" />
      </div>
    )
  }

  if (loadError) {
    return (
      <div className="mx-auto w-full max-w-3xl px-4 py-16">
        <EmptyState
          title="比赛加载失败"
          description={loadError}
          action={
            <div className="flex flex-wrap justify-center gap-2">
              <Button variant="outline" onClick={() => setReloadToken((value) => value + 1)}>
                重新加载
              </Button>
              <Button variant="ghost" asChild>
                <Link to="/contests">返回比赛列表</Link>
              </Button>
            </div>
          }
        />
      </div>
    )
  }

  if (!contest) {
    return (
      <EmptyState
        title={notFound ? '比赛不存在' : '无法显示比赛'}
        description="它可能已被删除，或者你没有查看权限。"
        action={
          <Button variant="outline" asChild>
            <Link to="/contests">返回比赛列表</Link>
          </Button>
        }
      />
    )
  }

  const phase = contestPhase(contest)

  const scoreFormat = contest.format === 'ioi' || contest.format === 'oi'
  const canPrepare = canPrepareContest(contest, clock)

  const registerLabel = registrationError
    ? '报名状态不可用'
    : registered
      ? '已报名'
      : ended
        ? '比赛已结束'
        : started
          ? '比赛已开始'
          : user && !contest.permissions.register
            ? contest.permissions.previewProblems
              ? '协作视角'
              : '暂无参赛资格'
            : '报名参赛'

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 px-4 py-6">
      <Card>
        <CardContent className="flex flex-col gap-3 pt-5">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-xl font-semibold tracking-tight">{contest.title}</h1>
            <Badge variant={phase.variant}>{phase.label}</Badge>
            <Badge variant="outline">
              {contest.format === 'icpc' ? 'ICPC' : contest.format.toUpperCase()}
            </Badge>
            {contest.visibility === 'password' ? (
              <Badge variant="outline">
                <Lock />
                需要密码
              </Badge>
            ) : null}
          </div>

          {contest.description ? (
            <p className="text-sm text-muted-foreground">{contest.description}</p>
          ) : null}

          <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-sm text-muted-foreground">
            <span className="inline-flex items-center gap-1.5">
              <CalendarClock className="size-4" />
              {formatDateTime(contest.beginAt)} — {formatDateTime(contest.endAt)}
            </span>
            {contest.freezeAt ? (
              <span className="inline-flex items-center gap-1.5">
                <Snowflake className="size-4" />
                封榜 {formatDateTime(contest.freezeAt)}
              </span>
            ) : null}
            {contest.format === 'icpc' ? (
              <span>每次未通过罚时 {contest.penaltyMinutes} 分钟</span>
            ) : null}
            {contest.feedback !== 'full' ? (
              <span>
                {contest.feedback === 'none' ? '比赛中不公布判定结果' : '比赛中只公布最终判定'}
              </span>
            ) : null}
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <Button
              disabled={
                Boolean(registrationError) ||
                registered ||
                started ||
                Boolean(user && !contest.permissions.register)
              }
              loading={registering}
              onClick={() => handleRegister()}
            >
              {registerLabel}
            </Button>
            <Link
              to={`/submissions?contest=${contest.publicId || contest.id}`}
              className="inline-flex items-center gap-1.5 text-sm text-primary hover:underline"
            >
              <ListChecks className="size-4" />
              比赛提交记录
            </Link>
            {isStaff ? (
              <Link
                to={`/contests/${contest.publicId || contest.id}/jury`}
                className="inline-flex items-center gap-1.5 text-sm text-primary hover:underline"
              >
                <Gavel className="size-4" />
                裁判台
              </Link>
            ) : null}
          </div>
          {registrationError ? (
            <div
              className="flex flex-wrap items-center gap-2 border-t border-border pt-3 text-sm text-destructive"
              role="alert"
            >
              <span>{registrationError}</span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setReloadToken((value) => value + 1)}
              >
                重试
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>

      {refreshError && (
        <div role="alert" className="flex flex-wrap items-center gap-3 text-sm text-destructive">
          <span>{refreshError}</span>
          <Button variant="outline" onClick={() => void refreshDetails()}>
            重试刷新
          </Button>
        </div>
      )}

      <Tabs value={activeTab} onValueChange={setActiveTab} className="flex flex-col gap-3">
        <TabsList>
          <TabsTrigger value="problems">题目</TabsTrigger>
          <TabsTrigger value="rankboard">{board?.frozen ? '榜单(已封榜)' : '实时榜单'}</TabsTrigger>
          {canSeeClarifications ? (
            <TabsTrigger value="clarifications">
              <MessagesSquare className="size-4" />
              答疑
            </TabsTrigger>
          ) : null}
          {contest.permissions.previewProblems && (
            <>
              <TabsTrigger value="settings">设置</TabsTrigger>
              <TabsTrigger value="composition">题目编排</TabsTrigger>
              <TabsTrigger value="access">协作权限</TabsTrigger>
            </>
          )}
        </TabsList>
        {contest.permissions.previewProblems && (
          <>
            <TabsContent value="settings" forceMount className="data-[state=inactive]:hidden">
              <ContestSettings
                contest={contest}
                canEdit={canPrepare}
                onSaved={() => void refreshDetails()}
              />
            </TabsContent>
            <TabsContent value="composition" forceMount className="data-[state=inactive]:hidden">
              <ContestComposition
                contest={contest}
                problems={problems}
                canEdit={canPrepare}
                onSaved={() => void refreshDetails()}
              />
            </TabsContent>
          </>
        )}
        {contest.permissions.previewProblems && (
          <TabsContent value="access">
            <ResourceCollaboration
              kind="contest"
              id={contest.id}
              ownerId={contest.ownerId}
              ownerName={contest.ownerName}
              manage={contest.permissions.manageAccess}
              transfer={contest.permissions.transfer}
              onChanged={() => void refreshDetails()}
            />
          </TabsContent>
        )}

        <TabsContent value="problems">
          <Card className="overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="w-14">#</TableHead>
                  <TableHead>题目</TableHead>
                  {scoreFormat ? <TableHead className="w-16 text-right">分值</TableHead> : null}
                  <TableHead className="w-20 text-right">难度</TableHead>
                  <TableHead className="hidden w-64 md:table-cell">标签</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {problems.length === 0 ? (
                  <TableEmpty colSpan={scoreFormat ? 5 : 4}>
                    {contest.permissions.edit
                      ? '尚未组题，请在本页「题目编排」中添加题目'
                      : contest.visibility === 'password' && !registered
                        ? '报名后可查看比赛题目'
                        : '暂未公布题目'}
                  </TableEmpty>
                ) : (
                  problems.map((problem, index) => (
                    <TableRow key={problem.problemId}>
                      <TableCell>
                        <span className="grid size-6 place-items-center rounded bg-muted font-mono text-xs font-semibold">
                          {problem.label || problemLetter(problem.sortOrder ?? index)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <Link
                          to={problemHref({
                            ...problem,
                            contestId: contest.id,
                            contestPublicId: contest.publicId,
                          })}
                          className="font-medium hover:text-primary"
                        >
                          {problem.title}
                        </Link>
                      </TableCell>
                      {scoreFormat ? (
                        <TableCell className="text-right tabular-nums text-muted-foreground">
                          {problem.points}
                        </TableCell>
                      ) : null}
                      <TableCell className="text-right tabular-nums text-muted-foreground">
                        {problem.difficulty}
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        <div className="flex flex-wrap gap-1">
                          {problem.tags?.map((tag) => (
                            <Badge key={tag} variant="outline">
                              {tag}
                            </Badge>
                          ))}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </Card>
        </TabsContent>

        <TabsContent value="rankboard">
          <Card className="overflow-hidden">
            {boardState.error && board ? (
              <div
                className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3 text-sm text-destructive"
                role="alert"
              >
                <span>{boardState.error}</span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setBoardReloadToken((value) => value + 1)}
                >
                  立即重试
                </Button>
              </div>
            ) : null}
            {!contest.rankboardVisible && !isStaff ? (
              <EmptyState icon={<EyeOff />} title="该比赛未公开榜单" />
            ) : contest.visibility === 'password' && !registered && !isStaff ? (
              <EmptyState icon={<Lock />} title="报名后可查看比赛榜单" />
            ) : (boardState.status === 'idle' || boardState.status === 'loading') && !board ? (
              <div className="flex flex-col gap-2 p-4" role="status" aria-label="榜单加载中">
                {Array.from({ length: 5 }, (_, index) => (
                  <Skeleton key={index} className="h-10 w-full" />
                ))}
              </div>
            ) : boardState.status === 'error' && !board ? (
              <EmptyState
                icon={<Trophy />}
                title="榜单加载失败"
                description={boardState.error ?? undefined}
                action={
                  <Button
                    variant="outline"
                    onClick={() => setBoardReloadToken((value) => value + 1)}
                  >
                    重新加载
                  </Button>
                }
              />
            ) : board && board.rows.length > 0 ? (
              <div className="p-4">
                <Scoreboard board={board} highlightUserId={user?.id} />
              </div>
            ) : (
              <EmptyState
                icon={<Trophy />}
                title="暂无榜单数据"
                description="有人通过题目后就会出现。"
              />
            )}
          </Card>
        </TabsContent>

        {canSeeClarifications ? (
          <TabsContent value="clarifications">
            <Clarifications
              contestId={contest.id}
              problems={problems}
              isJury={contest.permissions.reply}
              readAll={contest.permissions.viewJury}
              canAsk={registered && started && !ended && contest.permissions.submit}
            />
          </TabsContent>
        ) : null}
      </Tabs>

      <Dialog open={passwordOpen} onOpenChange={setPasswordOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>输入比赛密码</DialogTitle>
          </DialogHeader>
          <label htmlFor="contest-password" className="sr-only">
            比赛密码
          </label>
          <Input
            id="contest-password"
            autoFocus
            type="password"
            placeholder="比赛密码"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && password) void handleRegister(password)
            }}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setPasswordOpen(false)}>
              取消
            </Button>
            <Button
              disabled={!password.trim()}
              loading={registering}
              onClick={() => handleRegister(password)}
            >
              报名
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
