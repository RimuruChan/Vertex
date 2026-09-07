import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Plus, Search } from 'lucide-react'
import { Link, useNavigate } from '@/domain/navigation'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useDomain } from '@/domain/DomainContext'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import { Pagination } from '@/components/ui/pagination'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
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
import { apiError, formatDateTime } from '@/lib/format'
import { contestPayload, newContestDraft } from '@/components/contest/contest-form'

const PAGE_SIZE = 20
export default function AdminContestPage() {
  const { can } = useDomain(),
    api = useDomainAPI(),
    navigate = useNavigate(),
    toast = useToast(),
    active = useActiveRef()
  const [params, setParams] = useSearchParams(),
    keyword = params.get('keyword') ?? ''
  const number = Number(params.get('page') ?? 1),
    page = Number.isSafeInteger(number) && number > 0 ? number : 1
  const [query, setQuery] = useState(keyword),
    [open, setOpen] = useState(false),
    [draft, setDraft] = useState(newContestDraft),
    [saving, setSaving] = useState(false),
    [error, setError] = useState<string | null>(null)
  const load = useCallback(
    (signal: AbortSignal) =>
      api.getApiAdminContests({ page, size: PAGE_SIZE, keyword }, { signal }),
    [api, page, keyword],
  )
  const remote = useRemote(load)
  useEffect(() => setQuery(keyword), [keyword])
  async function create() {
    if (saving || !can('contest.create')) return
    let payload
    try {
      payload = contestPayload(draft)
    } catch (cause) {
      setError((cause as Error).message)
      return
    }
    setSaving(true)
    setError(null)
    try {
      const result = await api.postApiAdminContests(payload)
      if (!active.current) return
      toast.success('私有比赛已创建，请在详情中继续完善')
      navigate(`/contests/${result.publicId}?tab=settings`)
    } catch (cause) {
      if (active.current) setError(apiError(cause, '创建比赛失败'))
    } finally {
      if (active.current) setSaving(false)
    }
  }
  return (
    <div className="page-shell">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">比赛管理</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            查找你能协作的比赛，进入详情维护设置、编排与权限。
          </p>
        </div>
        {can('contest.create') && (
          <Button
            onClick={() => {
              setDraft(newContestDraft())
              setError(null)
              setOpen(true)
            }}
          >
            <Plus />
            创建比赛
          </Button>
        )}
      </div>
      <form
        className="flex max-w-xl gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          setParams(query.trim() ? { keyword: query.trim() } : {})
        }}
      >
        <Input
          aria-label="搜索管理比赛"
          placeholder="比赛名称或编号"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <Button variant="outline" type="submit">
          <Search />
          搜索
        </Button>
      </form>
      <Card className="overflow-hidden">
        {remote.loading ? (
          <Skeleton className="m-4 h-48" />
        ) : remote.error ? (
          <EmptyState
            title="比赛列表加载失败"
            description={remote.error}
            action={<Button onClick={remote.reload}>重试</Button>}
          />
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-20">编号</TableHead>
                  <TableHead>比赛</TableHead>
                  <TableHead>赛制</TableHead>
                  <TableHead>可见性</TableHead>
                  <TableHead className="hidden sm:table-cell">开始时间</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {remote.data?.items.length ? (
                  remote.data.items.map((contest) => (
                    <TableRow key={contest.id}>
                      <TableCell className="font-mono text-muted-foreground">
                        {contest.publicId}
                      </TableCell>
                      <TableCell>
                        <Link
                          className="font-medium hover:text-primary"
                          to={`/contests/${contest.publicId}`}
                        >
                          {contest.title}
                        </Link>
                      </TableCell>
                      <TableCell>{contest.format.toUpperCase()}</TableCell>
                      <TableCell>
                        {{ public: '公开', private: '私有', password: '密码赛' }[
                          contest.visibility
                        ] ?? contest.visibility}
                      </TableCell>
                      <TableCell className="hidden text-xs sm:table-cell">
                        {formatDateTime(contest.beginAt)}
                      </TableCell>
                      <TableCell className="text-right">
                        <Button variant="ghost" size="sm" asChild>
                          <Link to={`/contests/${contest.publicId}`}>进入详情</Link>
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableEmpty colSpan={6}>
                    <EmptyState
                      title={keyword ? '没有匹配的比赛' : '暂无可协作的比赛'}
                      description="只显示你拥有、协作或负责赛务的比赛。"
                    />
                  </TableEmpty>
                )}
              </TableBody>
            </Table>
            <Pagination
              page={page}
              size={PAGE_SIZE}
              total={remote.data?.total ?? 0}
              onChange={(value) =>
                setParams({ ...(keyword ? { keyword } : {}), page: String(value) })
              }
            />
          </>
        )}
      </Card>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!saving) setOpen(value)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>创建比赛</DialogTitle>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              void create()
            }}
          >
            <p className="text-sm text-muted-foreground">
              先创建私有比赛，题目编排、赛制细节与权限在详情页设置。
            </p>
            {error && (
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
            )}
            <fieldset disabled={saving} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="create-contest-title">比赛名称</Label>
                <Input
                  id="create-contest-title"
                  required
                  autoFocus
                  value={draft.title}
                  onChange={(e) => setDraft({ ...draft, title: e.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="create-contest-begin">开始时间</Label>
                <Input
                  id="create-contest-begin"
                  type="datetime-local"
                  required
                  value={draft.beginAt}
                  onChange={(e) => setDraft({ ...draft, beginAt: e.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="create-contest-end">结束时间</Label>
                <Input
                  id="create-contest-end"
                  type="datetime-local"
                  required
                  value={draft.endAt}
                  onChange={(e) => setDraft({ ...draft, endAt: e.target.value })}
                />
              </div>
            </fieldset>
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                type="button"
                disabled={saving}
                onClick={() => setOpen(false)}
              >
                取消
              </Button>
              <Button type="submit" loading={saving}>
                创建并进入详情
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
