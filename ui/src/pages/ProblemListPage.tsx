import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Search, SearchX, X } from 'lucide-react'
import { getApiProblems as listProblems, getApiTags as listTags } from '@/generated/api/vertex'
import type {
  DtoProblemResponse as Problem,
  DtoTagResponse as Tag,
  GetApiProblemsStatus,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import ProblemStatusIcon from '@/components/ProblemStatusIcon'
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
import { apiError, difficultyLabel, formatRatio } from '@/lib/format'
import { cn } from '@/lib/utils'

const PAGE_SIZE = 20
const ANY = 'any'

function parsePage(value: string | null): number {
  const page = Number(value)
  return Number.isSafeInteger(page) && page > 0 ? page : 1
}

/**
 * Filters live in the URL so a filtered view can be linked and survives a
 * reload — tag chips on the problem page link straight back into it.
 */
export default function ProblemListPage() {
  const { user, ready } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()

  const page = parsePage(searchParams.get('page'))
  const keyword = searchParams.get('keyword') ?? ''
  const tag = searchParams.get('tag') ?? ''
  const difficulty = searchParams.get('difficulty') ?? ''
  const status = searchParams.get('status') ?? ''

  const [problems, setProblems] = useState<Problem[]>([])
  const [total, setTotal] = useState(0)
  const [tags, setTags] = useState<Tag[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadToken, setReloadToken] = useState(0)
  const [search, setSearch] = useState(keyword)

  useEffect(() => setSearch(keyword), [keyword])

  function updateParams(changes: Record<string, string | undefined>) {
    const next = new URLSearchParams(searchParams)
    for (const [key, value] of Object.entries(changes)) {
      if (value) next.set(key, value)
      else next.delete(key)
    }
    // Any filter change invalidates the current page number.
    if (!('page' in changes)) next.delete('page')
    setSearchParams(next)
  }

  useEffect(() => {
    // Wait for the session to be restored: a request sent before the access
    // token exists comes back anonymous, which would show every problem as
    // unattempted even for a signed-in user.
    if (!ready) return
    const controller = new AbortController()
    setLoading(true)
    setLoadError(null)
    listProblems(
      {
        page,
        size: PAGE_SIZE,
        keyword: keyword || undefined,
        tag: tag || undefined,
        difficulty: difficulty ? Number(difficulty) : undefined,
        status: (status || undefined) as GetApiProblemsStatus | undefined,
      },
      { signal: controller.signal },
    )
      .then((result) => {
        if (controller.signal.aborted) return
        setProblems(result.items)
        setTotal(result.total)
      })
      .catch((error) => {
        if (controller.signal.aborted) return
        const message = apiError(error, '题库加载失败')
        setProblems([])
        setTotal(0)
        setLoadError(message)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, user?.id, page, keyword, tag, difficulty, status, reloadToken])

  useEffect(() => {
    listTags()
      .then((result) => setTags(result.items))
      .catch(() => setTags([]))
  }, [])

  const hasFilters = Boolean(keyword || tag || difficulty || status)

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-5 px-4 py-8 sm:px-6 sm:py-10">
      <header className="flex flex-wrap items-end justify-between gap-3 border-b border-border pb-6">
        <div>
          <p className="mb-2 text-xs text-muted-foreground">练习 / 题库</p>
          <h1 className="text-2xl font-semibold tracking-tight">题目</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {loading ? '正在加载…' : loadError ? '题目总数暂不可用' : `共 ${total} 道公开题目`}
          </p>
        </div>
      </header>

      <div className="flex flex-wrap items-center gap-2 border-b border-border pb-5">
        <form
          className="relative min-w-56 flex-1 sm:max-w-xs"
          onSubmit={(event) => {
            event.preventDefault()
            updateParams({ keyword: search.trim() || undefined })
          }}
        >
          <label htmlFor="problem-search" className="sr-only">
            搜索题目
          </label>
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            id="problem-search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="搜索标题或来源"
            className="pl-8"
          />
        </form>

        <Select
          value={difficulty || ANY}
          onValueChange={(value) => updateParams({ difficulty: value === ANY ? undefined : value })}
        >
          <SelectTrigger className="w-32" aria-label="按难度筛选">
            <SelectValue placeholder="难度" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY}>全部难度</SelectItem>
            {Array.from({ length: 10 }, (_, index) => index + 1).map((value) => (
              <SelectItem key={value} value={String(value)}>
                {difficultyLabel(value).label} · {value}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {user ? (
          <Select
            value={status || ANY}
            onValueChange={(value) => updateParams({ status: value === ANY ? undefined : value })}
          >
            <SelectTrigger className="w-32" aria-label="按进度筛选">
              <SelectValue placeholder="状态" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>全部状态</SelectItem>
              <SelectItem value="solved">已通过</SelectItem>
              <SelectItem value="attempted">尝试过</SelectItem>
              <SelectItem value="none">未尝试</SelectItem>
            </SelectContent>
          </Select>
        ) : null}

        {tags.length > 0 ? (
          <Select
            value={tag || ANY}
            onValueChange={(value) => updateParams({ tag: value === ANY ? undefined : value })}
          >
            <SelectTrigger className="w-40" aria-label="按标签筛选">
              <SelectValue placeholder="标签" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>全部标签</SelectItem>
              {tags.map((item) => (
                <SelectItem key={item.name} value={item.name}>
                  {item.name} ({item.problemCount})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null}

        {hasFilters ? (
          <Button variant="ghost" size="sm" onClick={() => setSearchParams(new URLSearchParams())}>
            <X />
            清除筛选
          </Button>
        ) : null}
      </div>

      {loading ? (
        <div className="flex flex-col gap-2 border-y border-border py-4">
          {Array.from({ length: 6 }, (_, index) => (
            <Skeleton key={index} className="h-10 w-full" />
          ))}
        </div>
      ) : loadError ? (
        <div className="border-y border-border py-10 text-center">
          <p className="text-sm text-muted-foreground">{loadError}</p>
          <Button
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => setReloadToken((value) => value + 1)}
          >
            重试
          </Button>
        </div>
      ) : problems.length === 0 ? (
        <EmptyState
          icon={<SearchX />}
          title="没有匹配的题目"
          description={hasFilters ? '试试放宽筛选条件。' : '题库还是空的。'}
        />
      ) : (
        <>
          <div className="hidden overflow-hidden border-y border-border sm:block">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="w-10" />
                  <TableHead>标题</TableHead>
                  <TableHead className="w-28">难度</TableHead>
                  <TableHead className="hidden w-64 lg:table-cell">标签</TableHead>
                  <TableHead className="w-20 text-right">提交</TableHead>
                  <TableHead className="w-24 text-right">通过率</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {problems.map((problem) => {
                  const difficultyStyle = difficultyLabel(problem.difficulty)
                  return (
                    <TableRow key={problem.id}>
                      <TableCell>
                        <ProblemStatusIcon status={problem.userStatus} />
                      </TableCell>
                      <TableCell className="font-medium">
                        <Link
                          to={`/problems/${problem.id}`}
                          className="underline-offset-4 hover:text-primary hover:underline focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        >
                          {problem.title}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <span className={cn('text-xs font-medium', difficultyStyle.className)}>
                          {difficultyStyle.label} · {problem.difficulty}
                        </span>
                      </TableCell>
                      <TableCell className="hidden lg:table-cell">
                        <div className="flex flex-wrap gap-1">
                          {problem.tags?.slice(0, 3).map((item) => (
                            <button
                              key={item}
                              type="button"
                              className="text-xs text-muted-foreground underline-offset-4 hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                              onClick={(event) => {
                                event.stopPropagation()
                                updateParams({ tag: item })
                              }}
                            >
                              {item}
                            </button>
                          ))}
                          {problem.tags && problem.tags.length > 3 ? (
                            <span className="text-xs text-muted-foreground">
                              +{problem.tags.length - 3}
                            </span>
                          ) : null}
                        </div>
                      </TableCell>
                      <TableCell className="text-right tabular-nums text-muted-foreground">
                        {problem.submissionCount}
                      </TableCell>
                      <TableCell className="text-right tabular-nums text-muted-foreground">
                        {formatRatio(problem.acceptedCount, problem.submissionCount)}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>

          <div className="divide-y divide-border border-y border-border sm:hidden">
            {problems.map((problem) => {
              const difficultyStyle = difficultyLabel(problem.difficulty)
              return (
                <article key={problem.id} className="flex gap-3 py-4">
                  <ProblemStatusIcon status={problem.userStatus} className="mt-1 size-4 shrink-0" />
                  <div className="min-w-0 flex-1">
                    <Link
                      to={`/problems/${problem.id}`}
                      className="font-medium hover:text-primary hover:underline"
                    >
                      {problem.title}
                    </Link>
                    <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                      <span className={difficultyStyle.className}>
                        {difficultyStyle.label} · {problem.difficulty}
                      </span>
                      <span>
                        {formatRatio(problem.acceptedCount, problem.submissionCount)} 通过
                      </span>
                      {problem.source ? <span className="truncate">{problem.source}</span> : null}
                    </div>
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

      {!user ? (
        <p className="text-center text-sm text-muted-foreground">
          <Link to="/login" className="text-primary hover:underline">
            登录
          </Link>{' '}
          后可以看到自己的做题进度。
        </p>
      ) : null}
    </div>
  )
}
