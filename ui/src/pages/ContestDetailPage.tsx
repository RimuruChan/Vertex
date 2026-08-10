import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { CalendarClock, EyeOff, ListChecks, Lock, Snowflake, Trophy } from 'lucide-react'
import {
  getApiContestsId as getContest,
  getApiContestsIdRankboard as getContestRankboard,
  getApiContestsIdRegistration as getContestRegistration,
  postApiContestsIdRegister as registerContest,
} from '@/generated/api/vertex'
import type {
  DtoContestProblemResponse as ContestProblem,
  DtoContestResponse as Contest,
  DtoRankboardResponse as Rankboard,
  DtoRankboardRowResponse as RankRow,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
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
import { useToast } from '@/components/ui/toast'
import { apiError, formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'

const RANKBOARD_POLL_MS = 5000

/** Contest problems are labelled A, B, C… everywhere they appear. */
function problemLetter(index: number): string {
  return String.fromCharCode(65 + index)
}

export default function ContestDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const toast = useToast()
  const { user } = useAuth()

  const [contest, setContest] = useState<Contest | null>(null)
  const [problems, setProblems] = useState<ContestProblem[]>([])
  const [board, setBoard] = useState<Rankboard | null>(null)
  const [registered, setRegistered] = useState(false)
  const [loading, setLoading] = useState(true)
  const [registering, setRegistering] = useState(false)
  const [passwordOpen, setPasswordOpen] = useState(false)
  const [password, setPassword] = useState('')

  const loadBoard = useCallback(async () => {
    if (!id) return
    try {
      setBoard(await getContestRankboard(id, {}))
    } catch {
      // The board may not exist yet; the tab shows an empty state instead.
    }
  }, [id])

  const load = useCallback(async () => {
    if (!id) return
    setLoading(true)
    try {
      const details = await getContest(id)
      setContest(details.contest)
      setProblems(details.problems)
      setRegistered(user ? (await getContestRegistration(id)).registered : false)
      if (details.contest.rankboardVisible) await loadBoard()
    } catch (error) {
      toast.error(apiError(error, '比赛加载失败'))
      setContest(null)
    } finally {
      setLoading(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, user?.id, loadBoard])

  useEffect(() => {
    void load()
  }, [load])

  // Keep the live board fresh while the contest is still running.
  const running =
    contest !== null &&
    contest.rankboardVisible &&
    Date.now() <= new Date(contest.endAt).getTime()
  useEffect(() => {
    if (!running) return
    const timer = window.setInterval(() => void loadBoard(), RANKBOARD_POLL_MS)
    return () => window.clearInterval(timer)
  }, [running, loadBoard])

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
      await load()
    } catch (error) {
      toast.error(apiError(error, '报名失败'))
    } finally {
      setRegistering(false)
    }
  }

  if (loading) {
    return (
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 px-4 py-6">
        <Skeleton className="h-28 w-full" />
        <Skeleton className="h-72 w-full" />
      </div>
    )
  }

  if (!contest) {
    return (
      <EmptyState
        title="比赛不存在"
        description="它可能已被删除,或者你没有查看权限。"
        action={
          <Button variant="outline" asChild>
            <Link to="/contests">返回比赛列表</Link>
          </Button>
        }
      />
    )
  }

  const now = Date.now()
  const started = now >= new Date(contest.beginAt).getTime()
  const ended = now > new Date(contest.endAt).getTime()
  const phase = contestPhase(contest)

  const registerLabel = registered
    ? '已报名'
    : ended
      ? '比赛已结束'
      : started
        ? '比赛已开始'
        : '报名参赛'

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 px-4 py-6">
      <Card>
        <CardContent className="flex flex-col gap-3 pt-5">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-xl font-semibold tracking-tight">{contest.title}</h1>
            <Badge variant={phase.variant}>{phase.label}</Badge>
            <Badge variant="outline">{contest.rule === 'acm' ? 'ACM/ICPC' : contest.rule.toUpperCase()}</Badge>
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
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <Button
              disabled={registered || started}
              loading={registering}
              onClick={() => handleRegister()}
            >
              {registerLabel}
            </Button>
            <Link
              to={`/submissions?contest=${contest.id}`}
              className="inline-flex items-center gap-1.5 text-sm text-primary hover:underline"
            >
              <ListChecks className="size-4" />
              比赛提交记录
            </Link>
          </div>
        </CardContent>
      </Card>

      <Tabs defaultValue="problems" className="flex flex-col gap-3">
        <TabsList>
          <TabsTrigger value="problems">题目</TabsTrigger>
          <TabsTrigger value="rankboard">
            {board?.frozen ? '榜单(已封榜)' : '实时榜单'}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="problems">
          <Card className="overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="w-14">#</TableHead>
                  <TableHead>题目</TableHead>
                  <TableHead className="w-20 text-right">难度</TableHead>
                  <TableHead className="hidden w-64 md:table-cell">标签</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {problems.length === 0 ? (
                  <TableEmpty colSpan={4}>
                    {user?.role === 'admin'
                      ? '尚未组题,请到比赛管理中添加题目'
                      : contest.visibility === 'password' && !registered
                        ? '报名后可查看比赛题目'
                        : '暂未公布题目'}
                  </TableEmpty>
                ) : (
                  problems.map((problem, index) => (
                    <TableRow key={problem.problemId}>
                      <TableCell>
                        <span className="grid size-6 place-items-center rounded bg-muted font-mono text-xs font-semibold">
                          {problemLetter(problem.sortOrder ?? index)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <Link
                          to={`/problems/${problem.problemId}?contest=${contest.id}`}
                          className="font-medium hover:text-primary"
                        >
                          {problem.title}
                        </Link>
                      </TableCell>
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
            {!contest.rankboardVisible ? (
              <EmptyState icon={<EyeOff />} title="该比赛未公开榜单" />
            ) : board && board.rows.length > 0 ? (
              <RankboardTable board={board} />
            ) : (
              <EmptyState icon={<Trophy />} title="暂无榜单数据" description="有人通过题目后就会出现。" />
            )}
          </Card>
        </TabsContent>
      </Tabs>

      <Dialog open={passwordOpen} onOpenChange={setPasswordOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>输入比赛密码</DialogTitle>
          </DialogHeader>
          <Input
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
            <Button loading={registering} onClick={() => handleRegister(password)}>
              报名
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

/** ACM rankboard: solved count, penalty, and one cell per problem. */
export function RankboardTable({ board }: { board: Rankboard }) {
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="w-14 text-center">排名</TableHead>
          <TableHead className="w-40">用户</TableHead>
          <TableHead className="w-16 text-center">通过</TableHead>
          <TableHead className="w-20 text-center">罚时</TableHead>
          {board.problemIds.map((problemId, index) => (
            <TableHead key={problemId} className="w-16 text-center">
              {problemLetter(index)}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {board.rows.map((row) => (
          <TableRow key={row.userId}>
            <TableCell className="text-center font-medium tabular-nums">{row.rank}</TableCell>
            <TableCell className="truncate">
              <Link to={`/users/${row.username}`} className="hover:text-primary">
                {row.username}
              </Link>
            </TableCell>
            <TableCell className="text-center font-medium tabular-nums">{row.solved}</TableCell>
            <TableCell className="text-center tabular-nums text-muted-foreground">
              {Math.floor(row.penalty / 60)}
            </TableCell>
            {board.problemIds.map((problemId, index) => (
              <TableCell key={problemId} className="text-center">
                <RankboardCell cell={row.cells[index]} />
              </TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function RankboardCell({ cell }: { cell: RankRow['cells'][number] | undefined }) {
  if (!cell) return <span className="text-muted-foreground/50">·</span>

  const base = 'inline-flex min-w-9 justify-center rounded px-1.5 py-0.5 text-xs tabular-nums'
  if (cell.solvedAt) {
    return (
      <span
        className={cn(base, 'bg-verdict-ac-bg font-medium text-verdict-ac')}
        title={`${cell.attempts} 次尝试`}
      >
        {Math.floor(cell.penaltySec / 60)}
      </span>
    )
  }
  if (cell.pendingCount > 0) {
    return <span className={cn(base, 'bg-verdict-tle-bg text-verdict-tle')}>?</span>
  }
  if (cell.attempts > 0) {
    return <span className={cn(base, 'text-verdict-wa')}>-{cell.attempts}</span>
  }
  return <span className="text-muted-foreground/50">·</span>
}
