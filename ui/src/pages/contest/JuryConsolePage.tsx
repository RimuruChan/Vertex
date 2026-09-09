import { useCallback, useEffect, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { Link, useNavigate } from '@/domain/navigation'
import { RefreshCw, ShieldCheck } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type {
  DtoContestProblemResponse as ContestProblem,
  DtoContestResponse as Contest,
  DtoRankboardResponse as Rankboard,
  DtoRejudgingChangeResponse as RejudgingChange,
  DtoRejudgingResponse as Rejudging,
  DtoSubmissionResponse as Submission,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import Clarifications from '@/components/contest/Clarifications'
import Scoreboard from '@/components/contest/Scoreboard'
import ProblemVersions from '@/components/contest/ProblemVersions'
import VerdictTag from '@/components/VerdictTag'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, PageSpinner, Progress } from '@/components/ui/misc'
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
export default function JuryConsolePage() {
  const {
    getApiAdminRejudgings: listRejudgings,
    getApiAdminRejudgingsIdChanges: listRejudgingChanges,
    getApiContestsId: getContest,
    getApiContestsIdRankboard: getRankboard,
    getApiSubmissions: listSubmissions,
    postApiAdminRejudgings: createRejudging,
    postApiAdminRejudgingsIdCancel: cancelRejudging,
  } = useDomainAPI()
  const { id = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const activeTab = params.get('tab') ?? 'board'
  const navigate = useNavigate()
  useEffect(() => {
    if (activeTab === 'staff') navigate(`/contests/${id}?tab=access`, { replace: true })
  }, [activeTab, id, navigate])
  const publicBoard = params.get('view') === 'public'
  const toast = useToast()
  const confirm = useConfirm()
  const { user } = useAuth()

  const [contest, setContest] = useState<Contest | null>(null)
  const [problems, setProblems] = useState<ContestProblem[]>([])
  const [board, setBoard] = useState<Rankboard | null>(null)
  const [submissions, setSubmissions] = useState<Submission[]>([])
  const [rejudgings, setRejudgings] = useState<Rejudging[]>([])
  const [loading, setLoading] = useState(true)
  const [denied, setDenied] = useState(false)
  const [workingError, setWorkingError] = useState<string | null>(null)
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

  const load = useCallback(async () => {
    setLoading(true)
    setDenied(false)
    setWorkingError(null)
    setRejudgingError(null)
    setRejudgingBlocked(false)
    try {
      const details = await getContest(id)
      const hiddenPublic =
        publicBoard &&
        details.contest.feedback === 'none' &&
        Date.now() <= Date.parse(details.contest.endAt)
      const scoreboard = hiddenPublic
        ? null
        : await getRankboard(id, publicBoard ? undefined : { view: 'jury' })
      setContest(details.contest)
      setProblems(details.problems)
      setBoard(scoreboard)
      setDenied(!details.contest.permissions.viewJury)
    } catch (error) {
      setDenied(true)
      toast.error(apiError(error, '无法进入裁判台'))
    } finally {
      setLoading(false)
    }
  }, [id, user?.id, toast, publicBoard])

  const loadWorking = useCallback(async () => {
    try {
      const subs = await listSubmissions({
        contest: id,
        problem: problemFilter === ANY ? undefined : problemFilter,
        status: statusFilter === ANY ? undefined : statusFilter,
        size: 50,
      })
      setSubmissions(subs.items)
      setWorkingError(null)
    } catch (error) {
      setWorkingError(apiError(error, '加载提交失败'))
      const status = responseStatus(error)
      if (status === 401 || status === 403) setDenied(true)
      return
    }

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
  }, [id, canViewRejudgings, problemFilter, rejudgingBlocked, statusFilter])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (loading || denied || !matchesReference(id, contest)) return
    let timer: number | undefined
    let stopped = false

    const poll = async () => {
      if (!document.hidden) {
        await loadWorking()
        if (activeTab === 'board') {
          try {
            const next = await getRankboard(id, publicBoard ? undefined : { view: 'jury' })
            if (!stopped) setBoard(next)
          } catch {
            /* The last successful board remains available; refresh can retry. */
          }
        }
      }
      if (!stopped) timer = window.setTimeout(poll, POLL_MS)
    }

    void poll()
    return () => {
      stopped = true
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [contest?.id, denied, id, loadWorking, loading, activeTab, publicBoard, getRankboard])

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
      await loadWorking()
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
      await loadWorking()
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
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <h1 className="text-xl font-semibold tracking-tight">
            {activeTab === 'board'
              ? '比赛榜单'
              : activeTab === 'clarifications'
                ? '公告与答疑'
                : activeTab === 'rejudge'
                  ? '重测批次'
                  : activeTab === 'staff'
                    ? '赛务人员'
                    : '比赛提交'}
          </h1>
          <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            <Badge variant="secondary">{contest.format.toUpperCase()}</Badge>
            <span>罚时 {contest.penaltyMinutes} 分钟/次</span>
            <span>
              {contest.feedback === 'full'
                ? '完整反馈'
                : contest.feedback === 'none'
                  ? '赛中不公布判定'
                  : '仅最终判定'}
            </span>
            {contest.freezeAt ? <span>封榜 {formatDateTime(contest.freezeAt)}</span> : null}
          </div>
        </div>
        <Button variant="outline" size="sm" onClick={() => void load()}>
          <RefreshCw />
          刷新
        </Button>
      </div>

      {activeTab === 'board' && (
        <div className="flex gap-2">
          <Button
            size="sm"
            variant={publicBoard ? 'outline' : 'default'}
            onClick={() => setParams({ tab: 'board' })}
          >
            内部实时
          </Button>
          <Button
            size="sm"
            variant={publicBoard ? 'default' : 'outline'}
            onClick={() => setParams({ tab: 'board', view: 'public' })}
          >
            公开视图
          </Button>
          <span className="self-center text-xs text-muted-foreground">
            {publicBoard ? '遵循公开榜单的封榜规则' : '内部数据，不受公开封榜影响'}
          </span>
        </div>
      )}
      {activeTab === 'rejudge' && (
        <Link to={`/contests/${id}/submissions`} className="text-sm text-primary">
          ← 本场提交记录
        </Link>
      )}

      <Tabs value={activeTab} className="flex flex-col gap-4">
        <TabsContent value="board">
          <Card className="p-4">
            {publicBoard &&
            contest.feedback === 'none' &&
            Date.now() <= Date.parse(contest.endAt) ? (
              <EmptyState
                title="赛后开放公开榜单"
                description="本场比赛不反馈判定，内部实时榜单仍可供赛务人员查看。"
              />
            ) : board ? (
              <Scoreboard board={board} />
            ) : null}
          </Card>
        </TabsContent>

        <TabsContent value="submissions">
          <Card className="overflow-hidden">
            {workingError ? (
              <div
                className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3 text-sm text-destructive"
                role="alert"
              >
                <span>{workingError}</span>
                <Button variant="outline" size="sm" onClick={() => void loadWorking()}>
                  重试
                </Button>
              </div>
            ) : null}
            <div className="flex flex-wrap items-center gap-2 border-b border-border p-4">
              <Select value={problemFilter} onValueChange={setProblemFilter}>
                <SelectTrigger className="w-48">
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
              <Select value={statusFilter} onValueChange={setStatusFilter}>
                <SelectTrigger className="w-48">
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
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="w-24">编号</TableHead>
                  <TableHead className="w-32">选手</TableHead>
                  <TableHead>题目</TableHead>
                  <TableHead className="w-40">判定</TableHead>
                  <TableHead className="w-20 text-right">分数</TableHead>
                  <TableHead className="w-44">时间</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {submissions.length === 0 ? (
                  <TableEmpty colSpan={6}>
                    <EmptyState title="没有匹配的提交" />
                  </TableEmpty>
                ) : (
                  submissions.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        <Link
                          to={`/contests/${id}/submissions/${item.publicId || item.id}`}
                          className="hover:underline"
                        >
                          #{shortId(item.id)}
                        </Link>
                      </TableCell>
                      <TableCell>{item.username}</TableCell>
                      <TableCell className="max-w-0 truncate">{item.problemTitle}</TableCell>
                      <TableCell>
                        <VerdictTag status={item.status} />
                      </TableCell>
                      <TableCell className="text-right tabular-nums">{item.score}</TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatDateTime(item.submittedAt)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
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
                  使用上方「提交」标签页的题目与判定筛选作为重测范围。重测会把选中的提交重新排队,
                  判完后榜单自动按新结果重算。
                </p>
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
