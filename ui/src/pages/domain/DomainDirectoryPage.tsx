import { useCallback, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Link, useNavigate } from '@/domain/navigation'
import {
  getApiDomains,
  postApiDomains,
  postApiDomainsDomainMembership,
} from '@/generated/api/vertex'
import type { DtoDomainResponse } from '@/generated/api/model'
import { useAuth } from '@/auth/AuthContext'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { domainRoleLabel } from '@/domain/labels'
import PageHeading from '@/components/PageHeading'
import { Button } from '@/components/ui/button'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import { Pagination } from '@/components/ui/pagination'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'

export default function DomainDirectoryPage() {
  const active = useActiveRef()
  const [params, setParams] = useSearchParams()
  const { user, ready } = useAuth(),
    navigate = useNavigate(),
    toast = useToast()
  const [query, setQuery] = useState(''),
    [keyword, setKeyword] = useState(''),
    [page, setPage] = useState(1),
    [busy, setBusy] = useState(false),
    [joining, setJoining] = useState<string | null>(null)
  const [slug, setSlug] = useState(''),
    [name, setName] = useState(''),
    [description, setDescription] = useState(''),
    [visibility, setVisibility] = useState<'public' | 'private'>('private'),
    [joinPolicy, setJoinPolicy] = useState<'open' | 'approval' | 'invite'>('invite')
  const open = !!user && params.get('create') === '1'
  function setOpen(value: boolean) {
    setParams(
      (current) => {
        const next = new URLSearchParams(current)
        if (value) next.set('create', '1')
        else next.delete('create')
        return next
      },
      { replace: true },
    )
  }
  const load = useCallback(
    (signal: AbortSignal) => getApiDomains({ page, size: 12, keyword }, { signal }),
    [page, keyword, user?.id, ready],
  )
  const remote = useRemote(load)
  async function create() {
    if (busy) return
    setBusy(true)
    try {
      const domain = await postApiDomains({ slug, name, description, visibility, joinPolicy })
      if (!active.current) return
      setOpen(false)
      navigate(`/d/${domain.slug}/settings`)
    } catch (error) {
      toast.error(apiError(error, '创建域失败'))
    } finally {
      setBusy(false)
    }
  }
  async function join(domain: DtoDomainResponse) {
    if (joining) return
    setJoining(domain.slug)
    try {
      const value = await postApiDomainsDomainMembership(domain.slug)
      toast.success(value.memberStatus === 'active' ? '已加入该域' : '申请已提交，等待审核')
      remote.reload()
    } catch (error) {
      toast.error(apiError(error, '加入失败'))
    } finally {
      setJoining(null)
    }
  }
  return (
    <div className="page-shell space-y-6">
      <PageHeading
        eyebrow="空间 / 域"
        title="找到你的学习与创作空间"
        description="题库、比赛与协作各自独立，账号在所有域中通用。"
        actions={
          user ? (
            <Button onClick={() => setOpen(true)}>创建域</Button>
          ) : (
            <Button asChild>
              <Link to="/login" state={{ from: '/domains?create=1' }}>
                登录后创建
              </Link>
            </Button>
          )
        }
      />
      <form
        className="flex max-w-lg gap-2"
        onSubmit={(event) => {
          event.preventDefault()
          setKeyword(query.trim())
          setPage(1)
        }}
      >
        <Input
          aria-label="搜索域"
          placeholder="搜索域名称或标识"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        <Button type="submit" variant="outline">
          搜索
        </Button>
      </form>
      {remote.error ? (
        <EmptyState
          title="域列表加载失败"
          description={remote.error}
          action={<Button onClick={remote.reload}>重试</Button>}
        />
      ) : remote.loading ? (
        <Skeleton className="h-64" />
      ) : remote.data?.items.length ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {remote.data.items.map((domain) => (
            <section key={domain.id} className="surface-panel flex min-w-0 flex-col gap-4 p-5">
              <div>
                <p className="text-xs text-muted-foreground">
                  {domain.official
                    ? '官方域'
                    : domain.visibility === 'private'
                      ? '私有域'
                      : '公开域'}{' '}
                  · {domain.slug}
                  {domain.archived ? ' · 已归档' : ''}
                </p>
                <h2 className="mt-2 break-words text-lg font-medium">{domain.name}</h2>
                <p className="mt-2 line-clamp-3 text-sm text-muted-foreground">
                  {domain.description || '这个空间还没有介绍。'}
                </p>
              </div>
              <p className="text-xs text-muted-foreground">
                {domain.memberStatus === 'active'
                  ? `域内角色：${domainRoleLabel(domain.memberRole)}${domain.ownerId === user?.id ? ' · owner' : ''}`
                  : domain.memberStatus === 'pending'
                    ? '申请待审核'
                    : domain.memberStatus === 'invited'
                      ? '你收到了一份邀请'
                      : domain.memberStatus === 'suspended'
                        ? '成员资格已停用'
                        : '尚未加入'}
              </p>
              <div className="mt-auto flex flex-wrap gap-2">
                {domain.canEnter && (
                  <Button size="sm" variant="outline" asChild>
                    <Link to={`/d/${domain.slug}`}>进入空间</Link>
                  </Button>
                )}
                {user &&
                  !domain.archived &&
                  !['active', 'suspended', 'pending'].includes(domain.memberStatus) &&
                  (domain.memberStatus === 'invited' || domain.joinPolicy !== 'invite') && (
                    <Button
                      size="sm"
                      loading={joining === domain.slug}
                      onClick={() => void join(domain)}
                    >
                      {domain.memberStatus === 'invited'
                        ? '接受邀请'
                        : domain.joinPolicy === 'approval'
                          ? '申请加入'
                          : '加入域'}
                    </Button>
                  )}
              </div>
            </section>
          ))}
        </div>
      ) : (
        <EmptyState title="没有找到可见的域" description="尝试更换关键词，或创建自己的空间。" />
      )}
      {!remote.error && remote.data && (
        <Pagination page={page} size={12} total={remote.data.total} onChange={setPage} />
      )}
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) setOpen(value)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>创建域</DialogTitle>
            <DialogDescription>标识创建后固定；域默认私有，只有受邀成员可加入。</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault()
              void create()
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="domain-slug">域标识</Label>
              <Input
                id="domain-slug"
                required
                pattern="[a-z][a-z0-9-]{1,31}"
                maxLength={32}
                placeholder="my-team"
                value={slug}
                onChange={(event) => setSlug(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="domain-name">显示名称</Label>
              <Input
                id="domain-name"
                required
                maxLength={100}
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="domain-description">介绍</Label>
              <Textarea
                id="domain-description"
                maxLength={8000}
                value={description}
                onChange={(event) => setDescription(event.target.value)}
              />
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="space-y-2 text-sm">
                可见性
                <select
                  aria-label="域可见性"
                  className="h-9 w-full rounded-md border bg-background px-2"
                  value={visibility}
                  onChange={(event) => setVisibility(event.target.value as typeof visibility)}
                >
                  <option value="private">私有</option>
                  <option value="public">公开</option>
                </select>
              </label>
              <label className="space-y-2 text-sm">
                加入方式
                <select
                  aria-label="域加入方式"
                  className="h-9 w-full rounded-md border bg-background px-2"
                  value={joinPolicy}
                  onChange={(event) => setJoinPolicy(event.target.value as typeof joinPolicy)}
                >
                  <option value="invite">邀请加入</option>
                  <option value="approval">申请审核</option>
                  <option value="open">开放加入</option>
                </select>
              </label>
            </div>
            <Button type="submit" loading={busy}>
              创建并进入设置
            </Button>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
