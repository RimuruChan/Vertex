import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Link } from '@/domain/navigation'
import { BookOpen, Lock, Search, SearchX, ThumbsUp } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type {
  DtoEditorialSummaryResponse as Editorial,
  GetApiEditorialsSort,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
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
import { apiError, formatRelative } from '@/lib/format'

const PAGE_SIZE = 15

function positivePage(value: string | null) {
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : 1
}

export default function EditorialListPage() {
  const { getApiEditorials: listEditorials } = useDomainAPI()
  const { user, ready } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()
  const page = positivePage(searchParams.get('page'))
  const keyword = searchParams.get('keyword') ?? ''
  const problem = searchParams.get('problem') ?? ''
  const sort: GetApiEditorialsSort = searchParams.get('sort') === 'votes' ? 'votes' : 'recent'
  const [query, setQuery] = useState(keyword)
  const [items, setItems] = useState<Editorial[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadToken, setReloadToken] = useState(0)
  const requestSequence = useRef(0)

  useEffect(() => setQuery(keyword), [keyword])

  useEffect(() => {
    if (!ready) return
    const sequence = ++requestSequence.current
    const controller = new AbortController()
    setLoading(true)
    setLoadError(null)
    listEditorials(
      {
        page,
        size: PAGE_SIZE,
        keyword: keyword || undefined,
        problem: problem || undefined,
        sort,
      },
      { signal: controller.signal },
    )
      .then((result) => {
        if (controller.signal.aborted || sequence !== requestSequence.current) return
        setItems(result.items)
        setTotal(result.total)
      })
      .catch((error) => {
        if (controller.signal.aborted || sequence !== requestSequence.current) return
        const message = apiError(error, '题解加载失败')
        setItems([])
        setTotal(0)
        setLoadError(message)
      })
      .finally(() => {
        if (!controller.signal.aborted && sequence === requestSequence.current) setLoading(false)
      })
    return () => controller.abort()
  }, [keyword, page, problem, ready, reloadToken, sort, user?.id])

  function update(changes: Record<string, string | undefined>) {
    const next = new URLSearchParams(searchParams)
    Object.entries(changes).forEach(([key, value]) => {
      if (value) next.set(key, value)
      else next.delete(key)
    })
    if (!('page' in changes)) next.delete('page')
    setSearchParams(next)
  }

  return (
    <div className="page-shell">
      <PageHeading
        eyebrow="社区 / 题解"
        title="思路，值得分享。"
        description="理解一种解法，也发现另一种可能。"
        actions={
          <Button variant="outline" asChild>
            <Link to="/problems">选一道题，写下思路</Link>
          </Button>
        }
      />

      <div className="filter-bar mb-5">
        <form
          className="relative min-w-64 flex-1"
          onSubmit={(event) => {
            event.preventDefault()
            update({ keyword: query.trim() || undefined })
          }}
        >
          <label htmlFor="editorial-search" className="sr-only">
            搜索题解
          </label>
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            id="editorial-search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索标题或内容"
            className="pl-9"
          />
        </form>
        <Select value={sort} onValueChange={(value) => update({ sort: value })}>
          <SelectTrigger className="w-32" aria-label="题解排序">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="recent">最新发布</SelectItem>
            <SelectItem value="votes">最多赞同</SelectItem>
          </SelectContent>
        </Select>
        {keyword ? (
          <Button variant="ghost" onClick={() => update({ keyword: undefined })}>
            清除搜索
          </Button>
        ) : null}
      </div>

      {problem ? (
        <div className="mb-4 flex items-center gap-2 text-sm text-muted-foreground">
          <span>
            只看题目 <span className="font-mono text-foreground">{problem}</span> 的题解
          </span>
          <Button variant="ghost" size="sm" onClick={() => update({ problem: undefined })}>
            清除
          </Button>
        </div>
      ) : null}

      {loading ? (
        <div className="divide-y divide-border" aria-label="正在加载题解">
          {Array.from({ length: 5 }, (_, index) => (
            <div key={index} className="py-5">
              <Skeleton className="mb-3 h-5 w-2/3" />
              <Skeleton className="h-4 w-full" />
            </div>
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
      ) : items.length === 0 ? (
        <EmptyState
          icon={keyword ? <SearchX /> : <BookOpen />}
          title={keyword ? '没有匹配的题解' : '还没有公开题解'}
          description={keyword ? '换个关键词试试。' : '完成一道题后，把关键思路写下来。'}
        />
      ) : (
        <div className="surface-panel divide-y divide-border overflow-hidden">
          {items.map((editorial) => (
            <article
              key={editorial.id}
              className="group px-5 py-6 transition-colors hover:bg-muted/30 sm:px-6"
            >
              <div className="flex items-start gap-4">
                <div className="min-w-0 flex-1">
                  <div className="mb-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                    {editorial.problemTitle ? (
                      <Link
                        to={`/problems/${editorial.problemPublicId || editorial.problemId}`}
                        className="hover:text-primary hover:underline"
                      >
                        {editorial.problemTitle}
                      </Link>
                    ) : null}
                    <span>{editorial.authorName || '匿名作者'}</span>
                    <span>{formatRelative(editorial.createdAt)}</span>
                    {editorial.locked ? (
                      <span className="inline-flex items-center gap-1">
                        <Lock className="size-3" /> 通过后可见
                      </span>
                    ) : null}
                  </div>
                  <h2 className="mt-3 text-lg font-semibold tracking-tight">
                    <Link
                      to={`/editorials/${editorial.publicId || editorial.id}`}
                      className="hover:text-primary hover:underline"
                    >
                      {editorial.title}
                    </Link>
                  </h2>
                  {editorial.locked ? (
                    <p className="mt-2 text-sm text-muted-foreground">
                      作者设置了防剧透，先独立完成题目再回来阅读。
                    </p>
                  ) : null}
                </div>
                <span
                  className="inline-flex shrink-0 items-center gap-1.5 rounded-md bg-muted px-2.5 py-1.5 text-xs tabular-nums text-muted-foreground"
                  aria-label={`${editorial.voteCount} 个赞同`}
                >
                  <ThumbsUp className="size-3.5" /> {editorial.voteCount}
                </span>
              </div>
            </article>
          ))}
        </div>
      )}

      <Pagination
        page={page}
        size={PAGE_SIZE}
        total={total}
        onChange={(next) => update({ page: String(next) })}
      />
    </div>
  )
}
