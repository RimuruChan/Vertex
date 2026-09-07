import { useEffect, useState } from 'react'
import { Link, useNavigate } from '@/domain/navigation'
import { ArrowRight, Plus, Search } from 'lucide-react'
import { useDomainAPI } from '@/domain/useDomainAPI'
import type { DtoProblemResponse } from '@/generated/api/model'
import PageHeading from '@/components/PageHeading'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'
import { useDomain } from '@/domain/DomainContext'

export default function AdminProblemPage() {
  const { can } = useDomain()
  const { getApiAdminProblems: listProblems, postApiAdminProblems: createProblem } = useDomainAPI()
  const navigate = useNavigate()
  const toast = useToast()
  const [items, setItems] = useState<DtoProblemResponse[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [query, setQuery] = useState('')
  const [keyword, setKeyword] = useState('')
  const [visibility, setVisibility] = useState('any')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [retry, setRetry] = useState(0)
  const [creating, setCreating] = useState(false)
  const [title, setTitle] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError(null)
    listProblems(
      {
        page,
        size: 20,
        keyword: keyword || undefined,
        visibility: visibility === 'any' ? undefined : visibility,
      },
      { signal: controller.signal },
    )
      .then((result) => {
        if (!controller.signal.aborted) {
          setItems(result.items)
          setTotal(result.total)
        }
      })
      .catch((caught) => {
        if (!controller.signal.aborted) setError(apiError(caught, '题目列表加载失败'))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [page, keyword, visibility, retry])

  async function create() {
    if (saving || !title.trim() || !can('problem.create')) return
    setSaving(true)
    try {
      const problem = await createProblem({ title: title.trim(), visibility: 'draft' })
      navigate(`/authoring/${problem.publicId || problem.id}`)
    } catch (caught) {
      toast.error(apiError(caught, '创建失败'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="page-shell">
      <PageHeading
        eyebrow="创作 / 题目"
        title="出题工作台"
        description="找到一份题目，进入详情继续完善。"
        actions={
          <Button
            disabled={!can('problem.create')}
            title={
              !can('problem.create') ? '当前域未授予创建权限；已有协作题目仍可进入' : undefined
            }
            onClick={() => setCreating(true)}
          >
            <Plus />
            新建题目
          </Button>
        }
      />
      <div className="filter-bar mb-5">
        <form
          className="flex min-w-0 flex-1 gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            setPage(1)
            setKeyword(query.trim())
          }}
        >
          <Input
            aria-label="搜索管理题目"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索题目名称或来源"
          />
          <Button type="submit" variant="secondary">
            <Search />
            搜索
          </Button>
        </form>
        <Select
          value={visibility}
          onValueChange={(value) => {
            setVisibility(value)
            setPage(1)
          }}
        >
          <SelectTrigger className="w-36" aria-label="题目可见性筛选">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="any">全部状态</SelectItem>
            <SelectItem value="draft">草稿</SelectItem>
            <SelectItem value="private">私有</SelectItem>
            <SelectItem value="public">公开</SelectItem>
          </SelectContent>
        </Select>
      </div>
      {loading ? (
        <Skeleton className="h-72" />
      ) : error ? (
        <EmptyState
          title="无法加载题目"
          description={error}
          action={<Button onClick={() => setRetry((value) => value + 1)}>重试</Button>}
        />
      ) : items.length === 0 ? (
        <EmptyState
          title="没有匹配的题目"
          description={
            can('problem.create')
              ? '试试其他关键词，或创建一份新题目。'
              : '当前域未授予创建权限；你拥有或获邀协作的题目会显示在这里。'
          }
        />
      ) : (
        <div className="surface-panel overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-16 sm:w-24">编号</TableHead>
                <TableHead>题目</TableHead>
                <TableHead className="hidden w-24 sm:table-cell">状态</TableHead>
                <TableHead className="hidden sm:table-cell">来源</TableHead>
                <TableHead className="w-12 sm:w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((problem) => (
                <TableRow key={problem.id}>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {problem.publicId}
                  </TableCell>
                  <TableCell>
                    <Link
                      to={`/authoring/${problem.publicId || problem.id}`}
                      className="font-medium hover:text-primary"
                    >
                      {problem.title}
                    </Link>
                    <span className="mt-1 block text-xs text-muted-foreground sm:hidden">
                      {{ draft: '草稿', private: '私有', public: '公开' }[problem.visibility] ||
                        problem.visibility}
                    </span>
                  </TableCell>
                  <TableCell className="hidden text-xs text-muted-foreground sm:table-cell">
                    {{ draft: '草稿', private: '私有', public: '公开' }[problem.visibility] ||
                      problem.visibility}
                  </TableCell>
                  <TableCell className="hidden text-xs text-muted-foreground sm:table-cell">
                    {problem.source}
                  </TableCell>
                  <TableCell>
                    <Button asChild size="sm" variant="ghost">
                      <Link to={`/authoring/${problem.publicId || problem.id}`}>
                        <span className="sr-only sm:not-sr-only">进入</span>
                        <ArrowRight />
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
      <Dialog open={creating} onOpenChange={(value) => !saving && setCreating(value)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>新建题目</DialogTitle>
            <DialogDescription>
              先取一个名字，题面、设置与测试数据都在详情中完善。
            </DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault()
              void create()
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="new-problem-name">题目名称</Label>
              <Input
                id="new-problem-name"
                autoFocus
                required
                maxLength={200}
                value={title}
                onChange={(event) => setTitle(event.target.value)}
              />
            </div>
            <Button type="submit" loading={saving} disabled={!title.trim()} className="w-full">
              创建并进入
            </Button>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
