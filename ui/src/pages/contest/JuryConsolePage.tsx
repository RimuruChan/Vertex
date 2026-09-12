import { useCallback, useEffect, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { Link } from '@/domain/navigation'
import { RefreshCw, ShieldCheck } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type {
  DtoContestProblemResponse as ContestProblem,
  DtoContestResponse as Contest,
  DtoRankboardResponse as Rankboard,
  DtoRejudgingChangeResponse as RejudgingChange,
  DtoRejudgingResponse as Rejudging,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import Clarifications from '@/components/contest/Clarifications'
import Scoreboard from '@/components/contest/Scoreboard'
import ProblemVersions from '@/components/contest/ProblemVersions'
import VerdictTag from '@/components/VerdictTag'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ContestPanel as Card, ContestPageHeader } from '@/components/contest/ContestPageLayout'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, PageSpinner, Progress, Skeleton } from '@/components/ui/misc'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent } from '@/components/ui/tabs'
import { useToast } from '@/components/ui/toast'
import { apiError, formatDateTime, shortId } from '@/lib/format'
import { matchesReference } from '@/lib/routes'

const ANY = 'any'
const POLL_MS = 5000

const VERDICTS = [
  'Accepted',
  'Wrong Answer',
  'Time Limit Exceeded',
  'Memory Limit Exceeded',
  'Runtime Error',
  'Compile Error',
  'System Error',
]

/**
 * The jury console. It is the one place a contest is actually run from:
 * live standings without the freeze, every submission and the clarification
 * queue. Resource capabilities distinguish jury mutations from observer reads.
 */
export default function JuryConsolePage({ activeTab }: { activeTab: string }) {
  const {
    getApiAdminRejudgings: listRejudgings,
    getApiAdminRejudgingsIdChanges: listRejudgingChanges,
    getApiContestsId: getContest,
    getApiContestsIdRankboard: getRankboard,
    postApiAdminRejudgings: createRejudging,
    postApiAdminRejudgingsIdCancel: cancelRejudging,
  } = useDomainAPI()
  const { id = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const publicBoard = params.get('view') === 'public'
  const toast = useToast()
  const confirm = useConfirm()
  const { user } = useAuth()

  const [contest, setContest] = useState<Contest | null>(null)
  const [problems, setProblems] = useState<ContestProblem[]>([])
  const [board, setBoard] = useState<Rankboard | null>(null)
  const [loadedPublicBoard, setLoadedPublicBoard] = useState(publicBoard)
  const [boardLoading, setBoardLoading] = useState(false)
  const [boardError, setBoardError] = useState<string | null>(null)
  const [boardRevision, setBoardRevision] = useState(0)
  const [rejudgings, setRejudgings] = useState<Rejudging[]>([])
  const [loading, setLoading] = useState(true)
  const [denied, setDenied] = useState(false)
  const [rejudgingError, setRejudgingError] = useState<string | null>(null)
  const [rejudgingBlocked, setRejudgingBlocked] = useState(false)

  const [problemFilter, setProblemFilter] = useState(ANY)
  const [statusFilter, setStatusFilter] = useState(ANY)
  const [rejudgeReason, setRejudgeReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [cancellingId, setCancellingId] = useState<string | null>(null)

  const [changes, setChanges] = useState<Record<string, RejudgingChange[]>>({})

  const canViewRejudgings = Boolean(contest?.permissions.viewJury)
  const canManageContest = Boolean(contest?.permissions.rejudge)

  const load = useCallback(
    async (signal?: AbortSignal) => {
      setLoading(true)
      setDenied(false)
      setRejudgingError(null)
      setRejudgingBlocked(false)
      setBoard(null)
      setBoardError(null)
      try {
        const details = await getContest(id, { signal })
        if (signal?.aborted) return
        setContest(details.contest)
        setProblems(details.problems)
        setDenied(!details.contest.permissions.viewJury)
      } catch (error) {
        if (signal?.aborted) return
        setDenied(true)
        toast.error(apiError(error, '无法进入裁判台'))
      } finally {
        if (!signal?.aborted) setLoading(false)
      }
    },
    [id, user?.id, toast, getContest],
  )

  const loadRejudgings = useCallback(async () => {
    if (!canViewRejudgings) {
      setRejudgings([])
      setRejudgingError(null)
      return
    }
    if (rejudgingBlocked) return

    try {
      const batches = await listRejudgings({ contest: id, limit: 10 })
      setRejudgings(batches.items)
      setRejudgingError(null)
    } catch (error) {
      setRejudgingError(apiError(error, '加载重测记录失败'))
      const status = responseStatus(error)
      if (status === 401 || status === 403) setRejudgingBlocked(true)
    }
  }, [id, canViewRejudgings, rejudgingBlocked])

  useEffect(() => {
    const controller = new AbortController()
    void load(controller.signal)
    return () => controller.abort()
  }, [load])

  useEffect(() => {
    if (loading || denied || !matchesReference(id, contest) || activeTab !== 'rejudge') return
    let timer: number | undefined
    let stopped = false

    const poll = async () => {
      if (!document.hidden) {
        await loadRejudgings()
      }
      if (!stopped) timer = window.setTimeout(poll, POLL_MS)
    }

    void poll()
    return () => {
      stopped = true
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [contest?.id, denied, id, loadRejudgings, loading, activeTab])

  useEffect(() => {
    if (activeTab !== 'board' || loading || denied || !matchesReference(id, contest)) return
    const controller = new AbortController()
    let timer: number | undefined

    const refresh = async (foreground: boolean) => {
      if (foreground) {
        setBoardLoading(true)
        setBoardError(null)
      }
      try {
        if (
          publicBoard &&
          contest?.feedback === 'none' &&
          Date.now() <= Date.parse(contest.endAt)
        ) {
          setBoard(null)
          setBoardError(null)
          return
        }
        const next = await getRankboard(id, publicBoard ? undefined : { view: 'jury' }, {
          signal: controller.signal,
        })
        if (!controller.signal.aborted) {
          setBoard(next)
          setLoadedPublicBoard(publicBoard)
          setBoardError(null)
        }
      } catch (error) {
        if (controller.signal.aborted) return
        if ([401, 403, 404].includes(responseStatus(error) ?? 0)) setBoard(null)
        setBoardError(apiError(error, '榜单更新失败，请重试'))
      } finally {
        if (!controller.signal.aborted && foreground) setBoardLoading(false)
      }
    }
    const poll = async () => {
      if (!document.hidden) await refresh(false)
      if (!controller.signal.aborted) timer = window.setTimeout(poll, POLL_MS)
    }
    void refresh(true).then(() => {
      if (!controller.signal.aborted) timer = window.setTimeout(poll, POLL_MS)
    })
    return () => {
      controller.abort()
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [
    activeTab,
    loading,
    denied,
    id,
    user?.id,
    contest?.id,
    contest?.feedback,
    contest?.endAt,
    publicBoard,
    boardRevision,
    getRankboard,
  ])

  async function handleRejudge() {
    if (busy || !canManageContest) return
    const problemScope =
      problemFilter === ANY
        ? '全部题目'
        : (problems.find((problem) => problem.problemId === problemFilter)?.title ?? '所选题目')
    const statusScope = statusFilter === ANY ? '全部判定' : statusFilter
    const accepted = await confirm({
      title: '确认批量重测？',
      description: `范围：${problemScope} · ${statusScope}。匹配的提交会重新排队，判定结果与比赛榜单可能改变，并占用判题资源。`,
      confirmLabel: '开始重测',
      destructive: true,
    })
    if (!accepted) return
    setBusy(true)
    try {
      const batch = await createRejudging({
        contestId: contest?.id,
        problemId: problemFilter === ANY ? undefined : problemFilter,
        status: statusFilter === ANY ? undefined : statusFilter,
        reason: rejudgeReason.trim() || undefined,
      })
      toast.success(`已排队重测 ${batch.total} 份提交`)
      setRejudgeReason('')
      await loadRejudgings()
    } catch (error) {
      toast.error(apiError(error, '重测失败'))
    } finally {
      setBusy(false)
    }
  }

  async function handleCancel(batch: Rejudging) {
    if (cancellingId !== null || !canManageContest) return
    const accepted = await confirm({
      title: `取消重测 #${shortId(batch.id)}？`,
      description:
        '尚未开始的任务会停止排队并恢复重测前的结果；已经完成或正在执行的判定不会回滚，榜单会保留已经产生的新结果。',
      confirmLabel: '取消重测',
      destructive: true,
    })
    if (!accepted) return
    setCancellingId(batch.id)
    try {
      await cancelRejudging(batch.id)
      toast.success('已取消未开始的重测并恢复原结果')
      await loadRejudgings()
    } catch (error) {
      toast.error(apiError(error, '取消失败'))
    } finally {
      setCancellingId(null)
    }
  }

  async function toggleChanges(batch: Rejudging) {
    if (changes[batch.id]) {
      setChanges((current) => {
        const next = { ...current }
        delete next[batch.id]
        return next
      })
      return
    }
    try {
      const result = await listRejudgingChanges(batch.id, { limit: 100 })
      setChanges((current) => ({ ...current, [batch.id]: result.items }))
    } catch (error) {
      toast.error(apiError(error, '加载改判列表失败'))
    }
  }

  function switchBoard(nextPublic: boolean) {
    if (nextPublic === publicBoard) {
      if (boardError) setBoardRevision((value) => value + 1)
      return
    }
    setParams(
      (current) => {
        const next = new URLSearchParams(current)
        if (nextPublic) next.set('view', 'public')
        else next.delete('view')
        return next
      },
      { preventScrollReset: true },
    )
  }
  // Keep the selected view and its description tied to the displayed response
  // until the requested view arrives, including after a failed switch.
  const displayedPublic = board ? loadedPublicBoard : publicBoard
  const switchingBoard = board !== null && displayedPublic !== publicBoard

  if (loading) return <PageSpinner />
  if (denied || !contest) {
    return (
      <div className="mx-auto w-full max-w-3xl px-4 py-16">
        <EmptyState
          icon={<ShieldCheck />}
          title="没有裁判权限"
          description="需要本场比赛的赛务读取权限，由比赛所有权、裁判/观察员授权或域资源管理权限提供。"
          action={
            <Button variant="outline" asChild>
              <Link to={`/contests/${id}`}>返回比赛</Link>
            </Button>
          }
        />
      </div>
    )
  }

  return (
    <div className="contest-page-shell flex flex-col gap-5">
      <ContestPageHeader
        title={
          activeTab === 'board'
            ? '比赛榜单'
            : activeTab === 'clarifications'
              ? '公告与答疑'
              : '重测'
        }
        description={
          activeTab === 'board'
            ? '查看本场排名与各题成绩。'
            : activeTab === 'clarifications'
              ? '查看比赛公告，与选手交流题目相关问题。'
              : '按范围重新评测提交，跟踪批次进度与改判记录。'
        }
        action={
          <Button
            variant="outline"
            size="sm"
            disabled={activeTab === 'board' && boardLoading}
            onClick={() =>
              activeTab === 'board' ? setBoardRevision((value) => value + 1) : void load()
            }
          >
            <RefreshCw
              className={
                activeTab === 'board' && boardLoading ? 'motion-safe:animate-spin' : undefined
              }
            />
            刷新
          </Button>
        }
      />

      <Tabs value={activeTab} className="flex flex-col gap-4">
        <TabsContent value="board">
          <Card className="overflow-hidden" aria-busy={boardLoading} aria-label="比赛榜单">
            <div className="flex flex-wrap items-center gap-2 border-b border-border bg-muted/15 px-5 py-3">
              <Button
                size="sm"
                variant={displayedPublic ? 'outline' : 'default'}
                aria-pressed={!displayedPublic}
                onClick={() => switchBoard(false)}
              >
                内部实时
              </Button>
              <Button
                size="sm"
                variant={displayedPublic ? 'default' : 'outline'}
                aria-pressed={displayedPublic}
                onClick={() => switchBoard(true)}
              >
                公开视图
              </Button>
              <span className="self-center text-xs text-muted-foreground" role="status">
                {boardLoading
                  ? switchingBoard
                    ? `正在切换到${publicBoard ? '公开视图' : '内部实时榜单'}…`
                    : '正在更新榜单…'
                  : displayedPublic
                    ? '遵循公开榜单的封榜规则'
                    : '内部数据，不受公开封榜影响'}
              </span>
            </div>
            {boardError && (
              <div
                role="alert"
                className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-5 py-3 text-sm text-destructive"
              >
                <span>
                  {boardError}
                  {board ? `，当前保留上次${displayedPublic ? '公开' : '内部'}榜单。` : ''}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={boardLoading}
                  onClick={() => setBoardRevision((value) => value + 1)}
                >
                  重试
                </Button>
              </div>
            )}
            {publicBoard &&
            contest.feedback === 'none' &&
            Date.now() <= Date.parse(contest.endAt) ? (
              <EmptyState
                title="赛后开放公开榜单"
                description="本场比赛不反馈判定，内部实时榜单仍可供赛务人员查看。"
              />
            ) : board ? (
              <Scoreboard board={board} highlightUserId={user?.id} />
            ) : boardError ? (
              <EmptyState title="榜单暂不可用" description="请重试加载当前视图。" />
            ) : (
              <div className="space-y-3 p-5" role="status" aria-label="榜单加载中">
                {Array.from({ length: 5 }, (_, index) => (
                  <Skeleton key={index} className="h-14" />
                ))}
              </div>
            )}
          </Card>
        </TabsContent>

        {canViewRejudgings ? (
          <TabsContent value="rejudge" className="flex flex-col gap-4">
            {canManageContest && (
              <ProblemVersions
                key={`${id}:${user?.id}`}
                contestId={id}
                problems={problems}
                onChanged={() => void load()}
              />
            )}
            {rejudgingError ? (
              <Card className="flex flex-wrap items-center justify-between gap-2 border-destructive/30 p-4 text-sm text-destructive">
                <span>{rejudgingError}</span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setRejudgingBlocked(false)
                    setRejudgingError(null)
                  }}
                >
                  重试
                </Button>
              </Card>
            ) : null}
            {canManageContest ? (
              <Card className="flex flex-col gap-3 p-4">
                <p className="text-sm font-medium">批量重测</p>
                <p className="text-xs text-muted-foreground">
                  在此选择需要重测的题目与原判定。匹配的提交会重新排队，判完后榜单自动按新结果重算。
                </p>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="flex min-w-0 flex-col gap-1.5">
                    <Label htmlFor="rejudge-problem">题目范围</Label>
                    <Select value={problemFilter} onValueChange={setProblemFilter} disabled={busy}>
                      <SelectTrigger id="rejudge-problem">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={ANY}>全部题目</SelectItem>
                        {problems.map((problem) => (
                          <SelectItem key={problem.problemId} value={problem.problemId}>
                            {problem.label} — {problem.title}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="flex min-w-0 flex-col gap-1.5">
                    <Label htmlFor="rejudge-status">原判定</Label>
                    <Select value={statusFilter} onValueChange={setStatusFilter} disabled={busy}>
                      <SelectTrigger id="rejudge-status">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={ANY}>全部判定</SelectItem>
                        {VERDICTS.map((verdict) => (
                          <SelectItem key={verdict} value={verdict}>
                            {verdict}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <div className="flex flex-wrap items-end gap-2">
                  <div className="flex flex-1 flex-col gap-1.5">
                    <Label htmlFor="rejudge-reason">原因</Label>
                    <Input
                      id="rejudge-reason"
                      value={rejudgeReason}
                      onChange={(event) => setRejudgeReason(event.target.value)}
                      placeholder="例如:修正了 checker"
                    />
                  </div>
                  <Button loading={busy} onClick={handleRejudge}>
                    <RefreshCw />
                    重测当前筛选
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  范围:
                  {problemFilter === ANY
                    ? '全部题目'
                    : problems.find((p) => p.problemId === problemFilter)?.label}
                  {' · '}
                  {statusFilter === ANY ? '全部判定' : statusFilter}
                </p>
              </Card>
            ) : (
              <p className="text-sm text-muted-foreground">
                当前为只读视角，可查看重测记录与改判详情。
              </p>
            )}

            <Card className="overflow-hidden">
              <p className="border-b border-border p-4 text-sm font-medium">重测记录</p>
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>原因</TableHead>
                    <TableHead className="w-28">状态</TableHead>
                    <TableHead className="w-48">进度</TableHead>
                    <TableHead className="w-20 text-right">改判</TableHead>
                    <TableHead className="w-44 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rejudgings.length === 0 ? (
                    <TableEmpty colSpan={5}>
                      <EmptyState title={rejudgingError ? '重测记录暂不可用' : '还没有重测记录'} />
                    </TableEmpty>
                  ) : (
                    rejudgings.map((batch) => (
                      <>
                        <TableRow key={batch.id}>
                          <TableCell className="max-w-0 truncate">
                            {batch.reason || '(未填写原因)'}
                            <div className="text-xs text-muted-foreground">
                              {formatDateTime(batch.createdAt)}
                            </div>
                          </TableCell>
                          <TableCell>
                            <Badge
                              variant={
                                batch.state === 'finished'
                                  ? 'success'
                                  : batch.state === 'cancelled'
                                    ? 'secondary'
                                    : 'warning'
                              }
                            >
                              {batch.state}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            <div className="flex items-center gap-2">
                              <Progress
                                value={batch.total > 0 ? (batch.done / batch.total) * 100 : 0}
                                className="w-24"
                              />
                              <span className="tabular-nums text-xs text-muted-foreground">
                                {batch.done} / {batch.total}
                              </span>
                            </div>
                          </TableCell>
                          <TableCell className="text-right tabular-nums">
                            {batch.changed > 0 ? (
                              <Badge variant="destructive">{batch.changed}</Badge>
                            ) : (
                              <span className="text-muted-foreground">0</span>
                            )}
                          </TableCell>
                          <TableCell>
                            <div className="flex justify-end gap-1">
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => toggleChanges(batch)}
                              >
                                {changes[batch.id] ? '收起' : '改判详情'}
                              </Button>
                              {canManageContest && batch.state === 'running' ? (
                                <Button
                                  size="sm"
                                  variant="ghost"
                                  disabled={cancellingId !== null}
                                  loading={cancellingId === batch.id}
                                  onClick={() => handleCancel(batch)}
                                >
                                  取消
                                </Button>
                              ) : null}
                            </div>
                          </TableCell>
                        </TableRow>
                        {changes[batch.id] ? (
                          <TableRow key={`${batch.id}-changes`} className="hover:bg-transparent">
                            <TableCell colSpan={5} className="bg-muted/40">
                              {changes[batch.id].length === 0 ? (
                                <p className="py-2 text-center text-sm text-muted-foreground">
                                  没有判定发生变化。
                                </p>
                              ) : (
                                <div className="flex flex-col gap-1 py-1">
                                  {changes[batch.id].map((change) => (
                                    <div
                                      key={change.submissionId}
                                      className="flex flex-wrap items-center gap-2 text-sm"
                                    >
                                      <Link
                                        to={`/contests/${id}/submissions/${change.submissionId}`}
                                        className="font-mono text-xs hover:underline"
                                      >
                                        #{shortId(change.submissionId)}
                                      </Link>
                                      <span className="text-muted-foreground">
                                        {change.username}
                                      </span>
                                      <span className="truncate">{change.problemTitle}</span>
                                      <span className="ml-auto flex items-center gap-2">
                                        <VerdictTag status={change.priorStatus} />
                                        <span className="text-muted-foreground">→</span>
                                        <VerdictTag status={change.status} />
                                      </span>
                                    </div>
                                  ))}
                                </div>
                              )}
                            </TableCell>
                          </TableRow>
                        ) : null}
                      </>
                    ))
                  )}
                </TableBody>
              </Table>
            </Card>
          </TabsContent>
        ) : null}

        <TabsContent value="clarifications">
          <Clarifications
            contestId={id}
            problems={problems}
            isJury={Boolean(contest?.permissions.reply)}
            readAll
            canAsk={false}
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function responseStatus(error: unknown): number | undefined {
  return (error as { response?: { status?: number } } | null)?.response?.status
}
