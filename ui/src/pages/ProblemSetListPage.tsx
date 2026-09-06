import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ListChecks, Lock, Plus, Search } from 'lucide-react'
import {
  getApiProblemSets as listSets,
  postApiProblemSets as createSet,
} from '@/generated/api/vertex'
import type {
  DtoSetResponse as ProblemSet,
  DtoSetUpsertRequestVisibility as SetVisibility,
} from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, Progress, Skeleton } from '@/components/ui/misc'
import { Pagination } from '@/components/ui/pagination'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/toast'
import { apiError, formatRelative } from '@/lib/format'

const PAGE_SIZE = 12

/** 题单列表: curated problem lists with the viewer's progress. */
export default function ProblemSetListPage() {
  const toast = useToast()
  const navigate = useNavigate()
  const { user, ready } = useAuth()
  const [sets, setSets] = useState<ProblemSet[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [keyword, setKeyword] = useState('')
  const requestKey = JSON.stringify([page, keyword, user?.id ?? '', user?.role ?? ''])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [loadedKey, setLoadedKey] = useState('')
  const [reloadToken, setReloadToken] = useState(0)

  const [creating, setCreating] = useState(false)
  const [saving, setSaving] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [visibility, setVisibility] = useState<SetVisibility>('public')

  useEffect(() => {
    if (!ready) return
    const controller = new AbortController()
    setLoading(true)
    setLoadError(null)
    listSets(
      { page, size: PAGE_SIZE, keyword: keyword || undefined },
      { signal: controller.signal },
    )
      .then((result) => {
        if (controller.signal.aborted) return
        setSets(result.items)
        setTotal(result.total)
        setLoadedKey(requestKey)
      })
      .catch((error) => {
        if (controller.signal.aborted) return
        setSets([])
        setTotal(0)
        setLoadError(apiError(error, '题单加载失败，请稍后重试'))
        setLoadedKey(requestKey)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [keyword, page, ready, reloadToken, requestKey, user?.id, user?.role])

  async function handleCreate() {
    if (!title.trim()) {
      toast.warning('请填写题单标题')
      return
    }
    setSaving(true)
    try {
      const created = await createSet({ title: title.trim(), description, visibility })
      toast.success('题单已创建')
      setCreating(false)
      setTitle('')
      setDescription('')
      navigate(`/problem-sets/${created.id}`)
    } catch (error) {
      toast.error(apiError(error, '创建失败'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="page-shell flex flex-col gap-5">
      <div className="mb-3 flex flex-wrap items-end justify-between gap-5">
        <div>
          <p className="eyebrow">练习 / 题单</p>
          <h1 className="text-[1.75rem] font-semibold tracking-tight sm:text-[2rem]">
            找到你的练习路线
          </h1>
          <p className="mt-3 text-sm text-muted-foreground">
            一个主题，一组题目。按照自己的节奏，一点点掌握。
          </p>
        </div>
        <div className="flex w-full items-center gap-2 sm:w-auto">
          <form
            className="relative min-w-0 flex-1 sm:w-52 sm:flex-none"
            onSubmit={(event) => {
              event.preventDefault()
              setPage(1)
              setKeyword(search.trim())
            }}
          >
            <label htmlFor="problem-set-search" className="sr-only">
              搜索题单
            </label>
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              id="problem-set-search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="搜索题单"
              className="pl-8"
            />
          </form>
          {user ? (
            <Button onClick={() => setCreating(true)}>
              <Plus />
              新建题单
            </Button>
          ) : null}
        </div>
      </div>

      {!ready || loading || loadedKey !== requestKey ? (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }, (_, index) => (
            <Skeleton key={index} className="h-36 w-full" />
          ))}
        </div>
      ) : loadError ? (
        <div className="border-y border-border">
          <EmptyState
            title="题单加载失败"
            description={loadError}
            action={
              <Button variant="outline" onClick={() => setReloadToken((value) => value + 1)}>
                重新加载
              </Button>
            }
          />
        </div>
      ) : sets.length === 0 ? (
        <Card>
          <EmptyState
            icon={<ListChecks />}
            title={keyword ? '没有匹配的题单' : '还没有题单'}
            description={
              keyword
                ? '换个关键词再试试。'
                : user
                  ? '创建第一个题单，把相关的题目收集起来。'
                  : '登录后可以创建自己的题单。'
            }
            action={
              keyword ? (
                <Button
                  variant="outline"
                  onClick={() => {
                    setSearch('')
                    setKeyword('')
                    setPage(1)
                  }}
                >
                  清除搜索
                </Button>
              ) : user ? (
                <Button onClick={() => setCreating(true)}>新建题单</Button>
              ) : undefined
            }
          />
        </Card>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {sets.map((item) => {
              const percent =
                item.problemCount > 0 ? (item.solvedCount / item.problemCount) * 100 : 0
              return (
                <Card
                  key={item.id}
                  className="flex flex-col gap-3 p-6 transition-colors hover:border-primary/40"
                >
                  <div className="flex items-start justify-between gap-2">
                    <Link
                      to={`/problem-sets/${item.id}`}
                      className="font-medium leading-snug hover:text-primary"
                    >
                      {item.title}
                    </Link>
                    {item.visibility === 'private' ? (
                      <Badge variant="secondary">
                        <Lock className="size-3" />
                        私有
                      </Badge>
                    ) : null}
                  </div>
                  {item.description ? (
                    <p className="line-clamp-2 text-sm text-muted-foreground">{item.description}</p>
                  ) : null}
                  <div className="mt-auto flex flex-col gap-1.5 pt-2">
                    <div className="flex items-center justify-between text-xs text-muted-foreground">
                      <span>
                        {item.problemCount} 题{user ? ` · 已过 ${item.solvedCount}` : ''}
                      </span>
                      <span>{item.authorName || '匿名'}</span>
                    </div>
                    {user ? <Progress value={percent} /> : null}
                    <span className="text-xs text-muted-foreground">
                      更新于 {formatRelative(item.updatedAt)}
                    </span>
                  </div>
                </Card>
              )
            })}
          </div>
          <Pagination page={page} size={PAGE_SIZE} total={total} onChange={setPage} />
        </>
      )}

      <Dialog open={creating} onOpenChange={setCreating}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>新建题单</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="set-title">标题</Label>
              <Input
                id="set-title"
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                placeholder="例如:动态规划入门 20 题"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="set-description">简介</Label>
              <Textarea
                id="set-description"
                rows={4}
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder="这个题单适合谁、按什么顺序做"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="set-visibility">可见性</Label>
              <Select
                value={visibility}
                onValueChange={(value) => setVisibility(value as SetVisibility)}
              >
                <SelectTrigger id="set-visibility">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="public">公开</SelectItem>
                  <SelectItem value="private">私有(仅自己可见)</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreating(false)}>
              取消
            </Button>
            <Button disabled={!title.trim()} loading={saving} onClick={handleCreate}>
              创建
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
