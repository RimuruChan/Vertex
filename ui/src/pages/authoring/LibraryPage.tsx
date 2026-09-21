import { useEffect, useState } from 'react'
import {
  ArrowRight,
  GitCommitHorizontal,
  LockKeyhole,
  Plus,
  RefreshCw,
  Search,
  Upload,
} from 'lucide-react'
import { Link, useNavigate } from '@/domain/navigation'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useDomain } from '@/domain/DomainContext'
import { useAuth } from '@/auth/AuthContext'
import type { DomainLibraryItem } from '@/generated/api/model'
import PageHeading from '@/components/PageHeading'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { apiError, formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'

function draftState(item: DomainLibraryItem) {
  if (item.hasConflict) return '待解决冲突'
  if (item.hasChanges) return '有未提交修改'
  if (item.hasCopy && item.headRevision > item.baseRevision) return '有共享更新'
  if (!item.canEdit) return '只读审阅'
  return item.hasCopy ? '副本已保存' : '尚未开始编辑'
}
const checkState: Record<string, string> = {
  queued: '排队中',
  running: '检查中',
  succeeded: '通过',
  failed: '失败',
  cancelled: '已取消',
}
export default function LibraryPage() {
  const { slug } = useDomain(),
    { user } = useAuth()
  return <Library key={`${slug}:${user?.id}`} />
}
function Library() {
  const api = useDomainAPI(),
    { can } = useDomain(),
    navigate = useNavigate()
  const [items, setItems] = useState<DomainLibraryItem[]>([]),
    [total, setTotal] = useState(0),
    [page, setPage] = useState(1)
  const [query, setQuery] = useState(''),
    [keyword, setKeyword] = useState(''),
    [visibility, setVisibility] = useState('all'),
    [status, setStatus] = useState('all')
  const [loading, setLoading] = useState(true),
    [error, setError] = useState(''),
    [retry, setRetry] = useState(0),
    [loaded, setLoaded] = useState(false)
  const [creating, setCreating] = useState(false),
    [importing, setImporting] = useState(false),
    [title, setTitle] = useState(''),
    [saving, setSaving] = useState(false),
    [createError, setCreateError] = useState('')
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    api
      .getApiAuthoringProblems(
        {
          page,
          size: 20,
          keyword: keyword || undefined,
          visibility: visibility === 'all' ? undefined : visibility,
          status: status === 'all' ? undefined : status,
        },
        { signal: controller.signal },
      )
      .then((value) => {
        if (controller.signal.aborted) return
        setItems(value.items)
        setTotal(value.total)
        setLoaded(true)
        if (!value.items.length && value.total > 0 && page > 1) setPage(Math.ceil(value.total / 20))
      })
      .catch((error) => {
        if (!controller.signal.aborted) setError(apiError(error, '出题列表加载失败'))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [api, page, keyword, visibility, status, retry])
  function start(importPackage: boolean) {
    setImporting(importPackage)
    setTitle(importPackage ? '导入的题目' : '')
    setCreateError('')
    setCreating(true)
  }
  async function create() {
    if (saving || !can('problem.create')) return
    if (!title.trim()) {
      setCreateError('填写题目名称。')
      return
    }
    setSaving(true)
    setCreateError('')
    try {
      const item = await api.postApiAdminProblems({ title: title.trim(), visibility: 'private' })
      navigate(`/authoring/${item.id}/${importing ? 'packages' : 'statement'}`)
    } catch (error) {
      setCreateError(apiError(error, '创建失败'))
    } finally {
      setSaving(false)
    }
  }
  return (
    <div className="min-w-0">
      <PageHeading
        eyebrow="工作台 / 出题"
        title="出题"
        description="继续自己的草稿，审阅共享提交，再检查和发布。"
        actions={
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" disabled={!can('problem.create')} onClick={() => start(true)}>
              <Upload />
              导入题包
            </Button>
            <Button disabled={!can('problem.create')} onClick={() => start(false)}>
              <Plus />
              新建题目
            </Button>
          </div>
        }
      />
      <div className="filter-bar mb-5">
        <form
          className="flex w-full min-w-0 gap-2 sm:flex-1"
          noValidate
          onSubmit={(event) => {
            event.preventDefault()
            setKeyword(query.trim())
            setPage(1)
          }}
        >
          <Input
            aria-label="搜索出题项目"
            placeholder="标题、题号或来源"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <Button type="submit" variant="outline">
            <Search />
            搜索
          </Button>
        </form>
        <Select
          value={status}
          onValueChange={(value) => {
            setStatus(value)
            setPage(1)
          }}
        >
          <SelectTrigger className="w-full sm:w-40" aria-label="出题状态筛选">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部进度</SelectItem>
            <SelectItem value="changes">我的未提交修改</SelectItem>
            <SelectItem value="conflicts">待解决冲突</SelectItem>
            <SelectItem value="unpublished">尚未发布</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={visibility}
          onValueChange={(value) => {
            setVisibility(value)
            setPage(1)
          }}
        >
          <SelectTrigger className="w-full sm:w-32" aria-label="题目可见性筛选">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部可见性</SelectItem>
            <SelectItem value="private">仅协作者</SelectItem>
            <SelectItem value="public">公开</SelectItem>
          </SelectContent>
        </Select>
        <Button
          variant="outline"
          size="icon"
          aria-label="刷新出题列表"
          disabled={loading}
          onClick={() => setRetry((value) => value + 1)}
        >
          <RefreshCw className={loading ? 'animate-spin' : undefined} />
        </Button>
      </div>
      {error && (
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/30 p-3">
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
          <Button variant="outline" onClick={() => setRetry((value) => value + 1)}>
            重试
          </Button>
        </div>
      )}
      {!loaded && loading ? (
        <Skeleton className="h-72" />
      ) : items.length === 0 ? (
        <EmptyState
          title="没有匹配的出题项目"
          description={
            can('problem.create')
              ? '创建题目或导入题包；你参与协作的题目也会出现在这里。'
              : '你拥有或获邀协作的题目会显示在这里。'
          }
        />
      ) : (
        <div
          className={cn(
            'overflow-hidden rounded-xl border bg-card transition-opacity',
            loading && 'opacity-60',
          )}
          aria-busy={loading}
        >
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-20">题号</TableHead>
                <TableHead>题目</TableHead>
                <TableHead className="hidden md:table-cell">我的副本</TableHead>
                <TableHead className="hidden lg:table-cell">共享提交</TableHead>
                <TableHead className="hidden xl:table-cell">检查</TableHead>
                <TableHead>发布</TableHead>
                <TableHead className="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.id}>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {item.id}
                  </TableCell>
                  <TableCell>
                    <Link
                      className="font-medium hover:text-primary"
                      to={`/authoring/${item.id}/${item.hasConflict ? 'changes' : 'statement'}`}
                    >
                      {item.title}
                    </Link>
                    <p className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                      {item.visibility !== 'public' && <LockKeyhole className="size-3" />}
                      <span>{item.visibility === 'public' ? '公开' : '仅协作者'}</span>
                      <span>· {item.ownerName}</span>
                      <span className="hidden 2xl:inline">· {formatDateTime(item.updatedAt)}</span>
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground md:hidden">
                      {draftState(item)}
                      {item.headRevision ? ` · r${item.headRevision}` : ''}
                    </p>
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    <span
                      className={cn(
                        'text-xs',
                        item.hasConflict
                          ? 'text-amber-700 dark:text-amber-400'
                          : item.hasChanges
                            ? 'text-primary'
                            : 'text-muted-foreground',
                      )}
                    >
                      {draftState(item)}
                    </span>
                    {item.hasCopy && item.baseRevision > 0 && (
                      <p className="mt-1 text-xs text-muted-foreground">
                        基于 r{item.baseRevision}
                      </p>
                    )}
                  </TableCell>
                  <TableCell className="hidden lg:table-cell">
                    {item.headRevision ? (
                      <Link
                        className="inline-flex items-center gap-1.5 text-xs hover:text-primary"
                        to={`/authoring/${item.id}/history`}
                      >
                        <GitCommitHorizontal className="size-3.5" />r{item.headRevision}
                      </Link>
                    ) : (
                      <span className="text-xs text-muted-foreground">尚无提交</span>
                    )}
                  </TableCell>
                  <TableCell className="hidden xl:table-cell">
                    {item.checkState ? (
                      <Link
                        to={`/authoring/${item.id}/checks`}
                        className={cn(
                          'text-xs hover:underline',
                          item.checkState === 'failed'
                            ? 'text-destructive'
                            : item.checkState === 'succeeded' && item.checkMatches
                              ? 'text-emerald-700 dark:text-emerald-400'
                              : 'text-muted-foreground',
                        )}
                      >
                        {checkState[item.checkState] ?? item.checkState}
                        {!item.checkMatches && (
                          <span className="mt-1 block text-muted-foreground">之前的材料</span>
                        )}
                      </Link>
                    ) : (
                      <span className="text-xs text-muted-foreground">尚未检查</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <Link
                      className="text-xs hover:text-primary"
                      to={`/authoring/${item.id}/releases`}
                    >
                      {item.publishedVersion ? `v${item.publishedVersion}` : '未发布'}
                    </Link>
                    {item.publishedRevision > 0 && (
                      <p className="mt-1 text-xs text-muted-foreground">
                        r{item.publishedRevision}
                      </p>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button asChild variant="outline" size="icon" className="size-8">
                      <Link
                        aria-label={`打开出题项目：${item.title}`}
                        to={`/authoring/${item.id}/${item.hasConflict ? 'changes' : 'statement'}`}
                      >
                        <ArrowRight className="size-4" />
                      </Link>
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <Pagination page={page} size={20} total={total} onChange={setPage} />
        </div>
      )}
      <Dialog
        open={creating}
        onOpenChange={(value) => {
          if (!saving) setCreating(value)
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{importing ? '导入题目' : '新建题目'}</DialogTitle>
            <DialogDescription>
              {importing
                ? '先创建私人题目，再选择题包预检。导入后的材料仍需确认并提交。'
                : '先取一个名字，题面、程序和测试数据可以逐步完善。'}
            </DialogDescription>
          </DialogHeader>
          <form
            noValidate
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault()
              void create()
            }}
          >
            <div className="space-y-2">
              <label htmlFor="new-authoring-title" className="text-sm font-medium">
                题目名称
              </label>
              <Input
                id="new-authoring-title"
                value={title}
                maxLength={200}
                onChange={(event) => {
                  setTitle(event.target.value)
                  setCreateError('')
                }}
                aria-invalid={Boolean(createError)}
                aria-describedby={createError ? 'authoring-create-error' : undefined}
              />
              {createError && (
                <p id="authoring-create-error" role="alert" className="text-sm text-destructive">
                  {createError}
                </p>
              )}
            </div>
            <Button type="submit" loading={saving} disabled={saving} className="w-full">
              {importing ? '继续选择题包' : '创建并开始编辑'}
            </Button>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
