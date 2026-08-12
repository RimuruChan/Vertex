import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { Inbox } from 'lucide-react'
import { getApiSubmissions as listSubmissions } from '@/generated/api/vertex'
import type { DtoSubmissionResponse as Submission } from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import VerdictTag, { isPendingVerdict } from '@/components/VerdictTag'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import { Pagination } from '@/components/ui/pagination'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useToast } from '@/components/ui/toast'
import {
  apiError,
  formatMemory,
  formatRelative,
  formatTime,
  languageLabel,
  shortId,
} from '@/lib/format'

const PAGE_SIZE = 20
const ANY = 'any'
const REFRESH_MS = 4000

const statuses = [
  'Accepted',
  'Wrong Answer',
  'Time Limit Exceeded',
  'Memory Limit Exceeded',
  'Runtime Error',
  'Compile Error',
]

export default function SubmissionListPage() {
  const navigate = useNavigate()
  const toast = useToast()
  const { user } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()

  const page = Math.max(1, Number(searchParams.get('page') ?? 1))
  const problem = searchParams.get('problem') ?? ''
  const status = searchParams.get('status') ?? ''
  const mine = searchParams.get('mine') === '1'

  const [submissions, setSubmissions] = useState<Submission[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)

  function updateParams(changes: Record<string, string | undefined>) {
    const next = new URLSearchParams(searchParams)
    for (const [key, value] of Object.entries(changes)) {
      if (value) next.set(key, value)
      else next.delete(key)
    }
    if (!('page' in changes)) next.delete('page')
    setSearchParams(next)
  }

  async function load(showSpinner: boolean) {
    if (showSpinner) setLoading(true)
    try {
      const result = await listSubmissions({
        page,
        size: PAGE_SIZE,
        problem: problem || undefined,
        status: status || undefined,
        user: mine && user ? user.username : undefined,
      })
      setSubmissions(result.items)
      setTotal(result.total)
    } catch (error) {
      if (showSpinner) toast.error(apiError(error, '提交记录加载失败'))
    } finally {
      if (showSpinner) setLoading(false)
    }
  }

  useEffect(() => {
    void load(true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, problem, status, mine])

  // While anything on screen is still being judged, refresh quietly in place.
  const hasPending = submissions.some((item) => isPendingVerdict(item.status))
  useEffect(() => {
    if (!hasPending) return
    const timer = window.setInterval(() => void load(false), REFRESH_MS)
    return () => window.clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasPending, page, problem, status, mine])

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">提交记录</h1>
          <p className="text-sm text-muted-foreground">共 {total} 条</p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant={mine ? 'default' : 'outline'}
            size="sm"
            onClick={() => updateParams({ mine: mine ? undefined : '1' })}
          >
            只看我的
          </Button>
          <Select
            value={status || ANY}
            onValueChange={(value) => updateParams({ status: value === ANY ? undefined : value })}
          >
            <SelectTrigger className="w-44">
              <SelectValue placeholder="判定" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>全部判定</SelectItem>
              {statuses.map((item) => (
                <SelectItem key={item} value={item}>
                  {item}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <Card className="overflow-hidden">
        {loading ? (
          <div className="flex flex-col gap-2 p-4">
            {Array.from({ length: 6 }, (_, index) => (
              <Skeleton key={index} className="h-10 w-full" />
            ))}
          </div>
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="w-24">编号</TableHead>
                  <TableHead className="w-32">判定</TableHead>
                  <TableHead>题目</TableHead>
                  <TableHead className="hidden w-32 sm:table-cell">提交者</TableHead>
                  <TableHead className="hidden w-28 md:table-cell">语言</TableHead>
                  <TableHead className="w-24 text-right">用时</TableHead>
                  <TableHead className="hidden w-28 text-right lg:table-cell">内存</TableHead>
                  <TableHead className="w-28 text-right">时间</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {submissions.length === 0 ? (
                  <TableEmpty colSpan={8}>
                    <EmptyState
                      icon={<Inbox />}
                      title="还没有提交记录"
                      description="去题库挑一道题开始吧。"
                      action={
                        <Button variant="outline" asChild>
                          <Link to="/problems">前往题库</Link>
                        </Button>
                      }
                    />
                  </TableEmpty>
                ) : (
                  submissions.map((submission) => (
                    <TableRow
                      key={submission.id}
                      className="cursor-pointer"
                      onClick={() => navigate(`/submissions/${submission.id}`)}
                    >
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        #{shortId(submission.id)}
                      </TableCell>
                      <TableCell>
                        <VerdictTag status={submission.status} />
                      </TableCell>
                      <TableCell className="max-w-0 truncate font-medium">
                        {submission.problemTitle}
                      </TableCell>
                      <TableCell className="hidden truncate sm:table-cell">
                        {submission.username}
                      </TableCell>
                      <TableCell className="hidden text-muted-foreground md:table-cell">
                        {languageLabel(submission.language)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums text-muted-foreground">
                        {formatTime(submission.totalTimeMs)}
                      </TableCell>
                      <TableCell className="hidden text-right tabular-nums text-muted-foreground lg:table-cell">
                        {formatMemory(submission.peakMemoryKb)}
                      </TableCell>
                      <TableCell
                        className="text-right text-xs text-muted-foreground"
                        title={submission.submittedAt}
                      >
                        {formatRelative(submission.submittedAt)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
            <Pagination
              page={page}
              size={PAGE_SIZE}
              total={total}
              onChange={(next) => updateParams({ page: String(next) })}
            />
          </>
        )}
      </Card>
    </div>
  )
}
