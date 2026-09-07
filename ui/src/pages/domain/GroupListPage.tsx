import { useCallback, useState } from 'react'
import { Link, useNavigate } from '@/domain/navigation'
import { useDomain } from '@/domain/DomainContext'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
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

export default function GroupListPage() {
  const active = useActiveRef()
  const { domain, can } = useDomain(),
    api = useDomainAPI(),
    navigate = useNavigate(),
    toast = useToast()
  const [query, setQuery] = useState(''),
    [keyword, setKeyword] = useState(''),
    [page, setPage] = useState(1),
    [open, setOpen] = useState(false),
    [name, setName] = useState(''),
    [description, setDescription] = useState(''),
    [owner, setOwner] = useState(domain.memberStatus === 'active' ? '' : domain.ownerName),
    [busy, setBusy] = useState(false)
  const load = useCallback(
    (signal: AbortSignal) => api.getApiGroups({ page, size: 20, keyword }, { signal }),
    [api, page, keyword],
  )
  const remote = useRemote(load)
  async function create() {
    if (busy || !can('domain.groups.manage')) return
    setBusy(true)
    try {
      const group = await api.postApiGroups({
        name,
        description,
        ownerUsername: owner.trim() || undefined,
      })
      if (!active.current) return
      navigate(`/groups/${group.publicId}`)
    } catch (error) {
      toast.error(apiError(error, '创建失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="page-shell space-y-5">
      <PageHeading
        eyebrow="域 / 协作"
        title="群组"
        description="组是域内的协作成员集合。资源授权给组后，成员变化会动态影响继承权限。"
        actions={
          can('domain.groups.manage') ? (
            <Button onClick={() => setOpen(true)}>创建群组</Button>
          ) : undefined
        }
      />
      <form
        className="flex max-w-lg gap-2"
        onSubmit={(event) => {
          event.preventDefault()
          setPage(1)
          setKeyword(query.trim())
        }}
      >
        <Input
          aria-label="搜索群组"
          placeholder="按组名称搜索"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        <Button type="submit" variant="outline">
          搜索
        </Button>
      </form>
      {remote.error ? (
        <EmptyState
          title="群组加载失败"
          description={remote.error}
          action={<Button onClick={remote.reload}>重试</Button>}
        />
      ) : !remote.data ? (
        <Skeleton className="h-56" />
      ) : remote.data.items.length ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {remote.data.items.map((group) => (
            <Link
              key={group.id}
              to={`/groups/${group.publicId}`}
              className="surface-panel space-y-2 p-5 transition-colors hover:border-primary/40"
            >
              <p className="text-xs text-muted-foreground">
                #{group.publicId} · {group.memberCount} 位有效成员
              </p>
              <h2 className="font-medium">{group.name}</h2>
              <p className="line-clamp-2 text-sm text-muted-foreground">
                {group.description || '暂无介绍'}
              </p>
              <p className="text-xs text-muted-foreground">
                owner：{group.ownerName}
                {group.viewerRole
                  ? ` · 我是${group.viewerRole === 'manager' ? '组管理者' : '组成员'}`
                  : ''}
              </p>
            </Link>
          ))}
        </div>
      ) : (
        <EmptyState title="没有匹配的群组" description="由域管理员创建组，再邀请本域成员加入。" />
      )}
      {remote.data && !remote.error && (
        <Pagination page={page} size={20} total={remote.data.total} onChange={setPage} />
      )}
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) setOpen(value)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>创建群组</DialogTitle>
            <DialogDescription>组所有者必须是当前域的有效成员。</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault()
              void create()
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="group-name">组名称</Label>
              <Input
                id="group-name"
                required
                maxLength={100}
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="group-description">组介绍</Label>
              <Textarea
                id="group-description"
                maxLength={8000}
                value={description}
                onChange={(event) => setDescription(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="group-owner">所有者用户名</Label>
              <Input
                id="group-owner"
                placeholder="留空表示自己"
                value={owner}
                onChange={(event) => setOwner(event.target.value)}
              />
            </div>
            <Button type="submit" loading={busy}>
              创建并进入群组
            </Button>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
