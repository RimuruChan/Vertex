import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { ArrowRight, BookOpen, Search, SearchX, Shuffle, X } from 'lucide-react'
import { getApiProblems as listProblems, getApiTags as listTags } from '@/generated/api/vertex'
import type {
  DtoProblemResponse as Problem,
  DtoTagResponse as Tag,
  GetApiProblemsStatus,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import ProblemStatusIcon from '@/components/ProblemStatusIcon'
import PageHeading from '@/components/PageHeading'
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
  const navigate = useNavigate()
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
    <div className="page-shell">
      <PageHeading
        eyebrow="练习 / 题库"
        title="题库"
        description="找到下一道值得思考的题目。"
        actions={
          <Button
            variant="outline"
            disabled={loading || !problems.length}
            onClick={() => {
              const chosen = problems[Math.floor(Math.random() * problems.length)]
              navigate(`/problems/${chosen.publicId || chosen.id}`)
            }}
          >
            <Shuffle /> 随机一题
          </Button>
        }
      />

      <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_232px]">
        <section className="min-w-0 space-y-4" aria-label="题目列表">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-1 rounded-lg bg-muted p-1" aria-label="做题进度">
              {(user
                ? [
                    ['', '全部题目'],
                    ['none', '未尝试'],
                    ['attempted', '尝试中'],
                    ['solved', '已通过'],
                  ]
                : [['', '全部题目']]
              ).map(([value, label]) => (
                <button
                  key={value}
                  type="button"
                  aria-pressed={status === value}
                  onClick={() => updateParams({ status: value || undefined })}
                  className={cn(
                    'rounded-md px-3 py-1.5 text-xs transition-colors',
                    status === value
                      ? 'bg-card font-medium text-foreground shadow-sm'
                      : 'text-muted-foreground hover:text-foreground',
                  )}
                >
                  {label}
                </button>
              ))}
            </div>
            <span className="text-xs tabular-nums text-muted-foreground" role="status">
              {loading ? '正在加载…' : loadError ? '题目总数暂不可用' : `${total} 道题目`}
            </span>
          </div>

          <div className="filter-bar">
            <form
              className="relative flex min-w-48 flex-1 gap-2"
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
                placeholder="搜索题目名称或来源…"
                className="min-w-0 pl-8"
              />
              <Button type="submit" variant="secondary" size="sm" className="h-9">
                搜索
              </Button>
            </form>

            <Select
              value={difficulty || ANY}
              onValueChange={(value) =>
                updateParams({ difficulty: value === ANY ? undefined : value })
              }
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
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setSearchParams(new URLSearchParams())}
              >
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
              <div className="surface-panel hidden overflow-hidden sm:block">
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
                              to={`/problems/${problem.publicId || problem.id}`}
                              className="block py-1 underline-offset-4 hover:text-primary hover:underline focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            >
                              {problem.title}
                            </Link>
                            <span className="text-xs font-normal text-muted-foreground">
                              {problem.source}
                            </span>
                          </TableCell>
                          <TableCell>
                            <span
                              className={cn(
                                'inline-flex rounded-md px-2 py-1 text-xs font-medium',
                                difficultyStyle.className,
                              )}
                            >
                              {difficultyStyle.label} · {problem.difficulty}
                            </span>
                          </TableCell>
                          <TableCell className="hidden lg:table-cell">
                            <div className="flex flex-wrap gap-1">
                              {problem.tags?.slice(0, 3).map((item) => (
                                <button
                                  key={item}
                                  type="button"
                                  className="rounded bg-muted/70 px-2 py-0.5 text-xs text-muted-foreground hover:bg-primary/8 hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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

              <div className="surface-panel divide-y divide-border px-4 sm:hidden">
                {problems.map((problem) => {
                  const difficultyStyle = difficultyLabel(problem.difficulty)
                  return (
                    <article key={problem.id} className="flex gap-3 py-4">
                      <ProblemStatusIcon
                        status={problem.userStatus}
                        className="mt-1 size-4 shrink-0"
                      />
                      <div className="min-w-0 flex-1">
                        <Link
                          to={`/problems/${problem.publicId || problem.id}`}
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
                          {problem.source ? (
                            <span className="truncate">{problem.source}</span>
                          ) : null}
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
        </section>
        <aside className="hidden space-y-5 xl:block">
          <section className="surface-panel p-5">
            <h2 className="mb-1 text-sm font-semibold">按知识点练习</h2>
            <p className="mb-4 text-xs leading-5 text-muted-foreground">一次专注一个方向。</p>
            <div className="flex flex-wrap gap-2">
              {tags.slice(0, 12).map((item) => (
                <button
                  key={item.name}
                  type="button"
                  aria-pressed={tag === item.name}
                  className={cn(
                    'rounded-md border px-2.5 py-1.5 text-xs transition-colors',
                    tag === item.name
                      ? 'border-primary/30 bg-primary/8 text-primary'
                      : 'border-border text-muted-foreground hover:border-primary/30 hover:text-primary',
                  )}
                  onClick={() => updateParams({ tag: tag === item.name ? undefined : item.name })}
                >
                  {item.name}
                </button>
              ))}
            </div>
          </section>
          <section className="rounded-xl bg-primary/5 p-5">
            <BookOpen className="mb-3 size-5 text-primary" />
            <h2 className="text-sm font-semibold">不知道从哪里开始？</h2>
            <p className="mt-2 text-xs leading-6 text-muted-foreground">
              跟着题单循序渐进，把知识点连起来。进度会随每一次通过自动更新。
            </p>
            <Link
              to="/problem-sets"
              className="mt-4 inline-flex items-center gap-2 text-xs font-medium text-primary"
            >
              探索题单 <ArrowRight className="size-3.5" />
            </Link>
          </section>
        </aside>
      </div>
    </div>
  )
}
