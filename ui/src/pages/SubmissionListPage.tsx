import { useCallback, useEffect, useState } from 'react'
import { useSearchParams, useParams, useLocation } from 'react-router-dom'
import { Link } from '@/domain/navigation'
import { Inbox, X } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type { DtoSubmissionResponse as Submission } from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import { submissionHref } from '@/lib/routes'
import { useContestSpace } from '@/components/contest/ContestContext'
import { useCanonicalPath } from '@/hooks/useCanonicalPath'
import VerdictTag, { isPendingVerdict } from '@/components/VerdictTag'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import { Pagination } from '@/components/ui/pagination'
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
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
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

function parsePage(value: string | null): number {
  const page = Number(value)
  return Number.isSafeInteger(page) && page > 0 ? page : 1
}

const statuses = [
  'Pending',
  'Judging',
  'Accepted',
  'Submitted',
  'Wrong Answer',
  'Time Limit Exceeded',
  'Memory Limit Exceeded',
  'Output Limit Exceeded',
  'Runtime Error',
  'Compile Error',
  'System Error',
]

const languages = ['cpp', 'c', 'python']

export default function SubmissionListPage() {
  const { contestId } = useParams()
  const space = useContestSpace()
  const location = useLocation()
  const { getApiSubmissions: listSubmissions } = useDomainAPI()
  const { user: viewer } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()

  const page = parsePage(searchParams.get('page'))
  const problem = searchParams.get('problem') ?? ''
  const contest = contestId ?? searchParams.get('contest') ?? ''
  useCanonicalPath(!contestId && contest ? `/contests/${contest}/submissions` : undefined, true)
  const language = searchParams.get('language') ?? ''
  const status = searchParams.get('status') ?? ''
  const requestedUser = searchParams.get('user')?.trim() ?? ''
  const mine =
    searchParams.get('mine') === '1' ||
    (searchParams.get('mine') === null &&
      !!contestId &&
      !!space?.details &&
      !space.details.contest.permissions.viewJury)
  const effectiveUser = mine ? viewer?.username : requestedUser || undefined

  const [submissions, setSubmissions] = useState<Submission[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)

  function updateParams(changes: Record<string, string | undefined>) {
    const next = new URLSearchParams(searchParams)
    for (const [key, value] of Object.entries(changes)) {
      if (value) next.set(key, value)
      else next.delete(key)
    }
    if (!('page' in changes)) next.delete('page')
    setSearchParams(next)
  }

  const load = useCallback(
    async (showSpinner: boolean, signal?: AbortSignal) => {
      if (showSpinner) {
        setLoading(true)
        setLoadError(null)
      }
      try {
        const result = await listSubmissions(
          {
            page,
            size: PAGE_SIZE,
            user: effectiveUser,
            problem: problem || undefined,
            contest: contest || undefined,
            language: language || undefined,
            status: status || undefined,
          },
          { signal },
        )
        if (signal?.aborted) return
        setSubmissions(result.items)
        setTotal(result.total)
      } catch (error) {
        if (!signal?.aborted && showSpinner) {
          const message = apiError(error, '提交记录加载失败')
          setSubmissions([])
          setTotal(0)
          setLoadError(message)
        }
      } finally {
        if (!signal?.aborted && showSpinner) setLoading(false)
      }
    },
    [contest, effectiveUser, language, page, problem, status],
  )

  useEffect(() => {
    const controller = new AbortController()
    void load(true, controller.signal)
    return () => controller.abort()
  }, [load])

  // While anything on screen is still being judged, refresh quietly in place.
  const hasPending = submissions.some((item) => isPendingVerdict(item.status))
  useEffect(() => {
    if (!hasPending) return
    const controller = new AbortController()
    let timer: number | undefined
    let stopped = false

    const poll = async () => {
      if (!document.hidden) await load(false, controller.signal)
      if (!stopped) timer = window.setTimeout(poll, REFRESH_MS)
    }

    timer = window.setTimeout(poll, REFRESH_MS)
    return () => {
      stopped = true
      controller.abort()
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [hasPending, load])

  const problemTitle = submissions.find((item) => item.problemId === problem)?.problemTitle
  const activeFilters: Array<{
    key: string
    label: string
    clear: () => void
  }> = []
  if (mine) {
    activeFilters.push({
      key: 'mine',
      label: '我的提交',
      clear: () => updateParams({ mine: '0', user: undefined }),
    })
  } else if (requestedUser) {
    activeFilters.push({
      key: 'user',
      label: `用户：${requestedUser}`,
      clear: () => updateParams({ user: undefined }),
    })
  }
  if (problem) {
    activeFilters.push({
      key: 'problem',
      label: `题目：${problemTitle || `#${shortId(problem)}`}`,
      clear: () => updateParams({ problem: undefined }),
    })
  }
  if (contest && !contestId) {
    activeFilters.push({
      key: 'contest',
      label: `比赛：#${shortId(contest)}`,
      clear: () => updateParams({ contest: undefined }),
    })
  }
  if (language) {
    activeFilters.push({
      key: 'language',
      label: `语言：${languageLabel(language)}`,
      clear: () => updateParams({ language: undefined }),
    })
  }
  if (status) {
    activeFilters.push({
      key: 'status',
      label: `判定：${status}`,
      clear: () => updateParams({ status: undefined }),
    })
  }

  return (
    <div className="page-shell flex flex-col gap-5">
      <header className="mb-2 flex flex-wrap items-end justify-between gap-5">
        <div>
          <p className="eyebrow">{contestId ? '本场比赛' : '评测 / 提交'}</p>
          <h1 className="text-[1.75rem] font-semibold tracking-tight sm:text-[2rem]">提交记录</h1>
          <p className="mt-3 text-sm text-muted-foreground">
            {loading ? '正在加载…' : loadError ? '提交总数暂不可用' : `共 ${total} 条`}
          </p>
        </div>
      </header>
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-border pb-4">
        <div
          role="group"
          aria-label="提交范围"
          className="inline-flex shrink-0 rounded-lg bg-muted p-1"
        >
          <Button
            size="sm"
            variant="ghost"
            aria-pressed={!mine}
            className={
              !mine ? 'bg-card text-foreground shadow-sm hover:bg-card' : 'text-muted-foreground'
            }
            onClick={() => updateParams({ mine: '0', user: undefined })}
          >
            全部提交
          </Button>
          <Button
            size="sm"
            variant="ghost"
            aria-pressed={mine}
            className={
              mine ? 'bg-card text-foreground shadow-sm hover:bg-card' : 'text-muted-foreground'
            }
            onClick={() => updateParams({ mine: '1', user: undefined })}
          >
            我的提交
          </Button>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {space?.details && (
            <Select
              value={problem || ANY}
              onValueChange={(value) =>
                updateParams({ problem: value === ANY ? undefined : value })
              }
            >
              <SelectTrigger className="w-40" aria-label="按比赛题目筛选">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ANY}>全部题目</SelectItem>
                {space.details.problems.map((item) => (
                  <SelectItem key={item.problemId} value={item.problemId}>
                    {item.label} · {item.title}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}

          <Select
            value={language || ANY}
            onValueChange={(value) => updateParams({ language: value === ANY ? undefined : value })}
          >
            <SelectTrigger className="w-36" aria-label="按语言筛选">
              <SelectValue placeholder="语言" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>全部语言</SelectItem>
              {languages.map((item) => (
                <SelectItem key={item} value={item}>
                  {languageLabel(item)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={status || ANY}
            onValueChange={(value) => updateParams({ status: value === ANY ? undefined : value })}
          >
            <SelectTrigger className="w-44" aria-label="按判定筛选">
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

      {contestId && space?.details?.contest.permissions.viewJury && (
        <div className="flex gap-3 text-sm">
          <span className="font-medium text-primary">提交记录</span>
          <Link
            className="text-muted-foreground hover:text-primary"
            to={`/contests/${contestId}/jury?tab=rejudge`}
          >
            重测批次
          </Link>
        </div>
      )}
      {contestId && space?.details?.contest.permissions.viewJury && (
        <form
          className="flex max-w-md gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            const value = new FormData(event.currentTarget).get('contestant')?.toString().trim()
            updateParams({ user: value || undefined, mine: '0' })
          }}
        >
          <Input
            key={requestedUser}
            name="contestant"
            defaultValue={requestedUser}
            aria-label="参赛者用户名"
            placeholder="按参赛者用户名筛选"
          />
          <Button type="submit" variant="outline">
            筛选
          </Button>
        </form>
      )}
      {activeFilters.length > 0 ? (
        <div
          className="flex flex-wrap items-center gap-1 border-b border-border pb-4"
          aria-label="当前筛选条件"
        >
          <span className="text-xs text-muted-foreground">当前筛选</span>
          {activeFilters.map((filter) => (
            <Button
              key={filter.key}
              type="button"
              variant="ghost"
              size="sm"
              onClick={filter.clear}
              aria-label={`清除筛选：${filter.label}`}
            >
              {filter.label}
              <X />
            </Button>
          ))}
          {activeFilters.length > 1 ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => setSearchParams(new URLSearchParams())}
            >
              清除全部
            </Button>
          ) : null}
        </div>
      ) : null}

      {loading ? (
        <div className="flex flex-col gap-2 border-y border-border py-4">
          {Array.from({ length: 6 }, (_, index) => (
            <Skeleton key={index} className="h-10 w-full" />
          ))}
        </div>
      ) : loadError ? (
        <div className="border-y border-border py-10 text-center">
          <p className="text-sm text-muted-foreground">{loadError}</p>
          <Button variant="outline" size="sm" className="mt-3" onClick={() => void load(true)}>
            重试
          </Button>
        </div>
      ) : submissions.length === 0 ? (
        <EmptyState
          icon={<Inbox />}
          title="还没有提交记录"
          description={contestId ? '本场暂无符合条件的提交。' : '去题库挑一道题开始吧。'}
          action={
            <Button variant="outline" asChild>
              <Link to={contestId ? `/contests/${contestId}?tab=problems` : '/problems'}>
                {contestId ? '前往比赛题目' : '前往题库'}
              </Link>
            </Button>
          }
        />
      ) : (
        <>
          <div className="surface-panel hidden overflow-hidden md:block">
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
                {submissions.map((submission) => {
                  const pending = isPendingVerdict(submission.status)
                  return (
                    <TableRow key={submission.id}>
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        <Link
                          to={submissionHref(submission)}
                          state={{ contestReturn: location.pathname + location.search }}
                          className="underline-offset-4 hover:text-primary hover:underline focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        >
                          #{shortId(submission.id)}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <VerdictTag status={submission.status} />
                      </TableCell>
                      <TableCell className="max-w-0 truncate font-medium">
                        <Link
                          to={submissionHref(submission)}
                          state={{ contestReturn: location.pathname + location.search }}
                          className="underline-offset-4 hover:text-primary hover:underline focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        >
                          {submission.problemTitle}
                        </Link>
                      </TableCell>
                      <TableCell className="hidden truncate sm:table-cell">
                        {submission.username}
                      </TableCell>
                      <TableCell className="hidden text-muted-foreground md:table-cell">
                        {languageLabel(submission.language)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums text-muted-foreground">
                        {pending ? '—' : formatTime(submission.totalTimeMs)}
                      </TableCell>
                      <TableCell className="hidden text-right tabular-nums text-muted-foreground lg:table-cell">
                        {pending ? '—' : formatMemory(submission.peakMemoryKb)}
                      </TableCell>
                      <TableCell
                        className="text-right text-xs text-muted-foreground"
                        title={submission.submittedAt}
                      >
                        {formatRelative(submission.submittedAt)}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>

          <div className="surface-panel divide-y divide-border px-4 md:hidden">
            {submissions.map((submission) => {
              const pending = isPendingVerdict(submission.status)
              return (
                <article key={submission.id} className="py-4">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <Link
                        to={submissionHref(submission)}
                        state={{ contestReturn: location.pathname + location.search }}
                        className="block truncate font-medium hover:text-primary hover:underline"
                      >
                        {submission.problemTitle}
                      </Link>
                      <p className="mt-1 font-mono text-xs text-muted-foreground">
                        #{shortId(submission.id)}
                      </p>
                    </div>
                    <VerdictTag status={submission.status} />
                  </div>
                  <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                    <span>{submission.username}</span>
                    <span>{languageLabel(submission.language)}</span>
                    <span>{pending ? '评测中' : formatTime(submission.totalTimeMs)}</span>
                    <span>{formatRelative(submission.submittedAt)}</span>
                  </div>
                </article>
              )
            })}
          </div>
          <Pagination
            page={page}
            size={PAGE_SIZE}
            total={total}
            onChange={(next) => updateParams({ page: String(next) })}
          />
        </>
      )}
    </div>
  )
}
