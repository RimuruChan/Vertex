import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useCallback, useState } from 'react'
import type { DtoMemberResponse } from '@/generated/api/model'
import { useDomain } from '@/domain/DomainContext'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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

const statuses: Record<string, string> = {
  active: '有效',
  pending: '待审核',
  invited: '已邀请',
  suspended: '已停用',
}
export default function DomainMembersPage() {
  const active = useActiveRef()
  const { domain, can, refresh } = useDomain(),
    api = useDomainAPI(),
    toast = useToast(),
    confirm = useConfirm()
  const [query, setQuery] = useState(''),
    [keyword, setKeyword] = useState(''),
    [page, setPage] = useState(1),
    [editing, setEditing] = useState<DtoMemberResponse | null>(null),
    [open, setOpen] = useState(false),
    [username, setUsername] = useState(''),
    [role, setRole] = useState('member'),
    [status, setStatus] = useState<'active' | 'pending' | 'invited' | 'suspended'>('invited'),
    [busy, setBusy] = useState(false)
  const load = useCallback(
    async (signal: AbortSignal) => {
      const [members, roles] = await Promise.all([
        api.getApiMembers({ page, size: 20, keyword }, { signal }),
        api.getApiRoles({ signal }),
      ])
      return { members, roles: roles.items }
    },
    [api, page, keyword],
  )
  const remote = useRemote(load)
  function edit(member: DtoMemberResponse | null) {
    setEditing(member)
    setUsername(member?.username ?? '')
    setRole(member?.roleKey ?? 'member')
    setStatus((member?.status as typeof status) ?? 'invited')
    setOpen(true)
  }
  async function save() {
    if (busy || !can('domain.members.manage')) return
    if (
      status === 'suspended' &&
      !(await confirm({
        title: `停用 ${username} 的域成员资格？`,
        description: '该账号将立即失去域内访问和继承的 group 权限；站点账号不受影响。',
        confirmLabel: '停用成员',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await api.putApiMembersUsername(username.trim(), { roleKey: role, status })
      setOpen(false)
      remote.reload()
      await refresh()
      toast.success('成员设置已保存')
    } catch (error) {
      toast.error(apiError(error, '保存失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <section className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <form
          className="flex gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            setPage(1)
            setKeyword(query.trim())
          }}
        >
          <Input
            aria-label="搜索域成员"
            placeholder="按用户名搜索"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <Button type="submit" variant="outline">
            搜索
          </Button>
        </form>
        {can('domain.members.manage') && <Button onClick={() => edit(null)}>邀请或添加成员</Button>}
      </div>
      {remote.error ? (
        <EmptyState
          title="成员列表加载失败"
          description={remote.error}
          action={<Button onClick={remote.reload}>重试</Button>}
        />
      ) : !remote.data ? (
        <Skeleton className="h-56" />
      ) : (
        <>
          <div className="surface-panel overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b bg-muted/30">
                <tr>
                  <th className="p-3">成员</th>
                  <th className="p-3">域角色</th>
                  <th className="p-3">状态</th>
                  <th className="p-3">
                    <span className="sr-only">操作</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {remote.data.members.items.map((member) => (
                  <tr key={member.userId} className="border-b last:border-0">
                    <td className="p-3">
                      {member.username}
                      {member.userId === domain.ownerId && (
                        <span className="ml-2 text-xs text-muted-foreground">owner</span>
                      )}
                    </td>
                    <td className="p-3">
                      {remote.data!.roles.find((role) => role.key === member.roleKey)?.name ??
                        member.roleKey}
                    </td>
                    <td className="p-3">{statuses[member.status] ?? member.status}</td>
                    <td className="p-3 text-right">
                      {can('domain.members.manage') && (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={member.userId === domain.ownerId}
                          onClick={() => edit(member)}
                        >
                          设置成员
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!remote.data.members.items.length && <EmptyState title="没有匹配的成员" />}
          </div>
          <Pagination page={page} size={20} total={remote.data.members.total} onChange={setPage} />
        </>
      )}
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) setOpen(value)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? '设置域成员' : '邀请或添加成员'}</DialogTitle>
            <DialogDescription>
              只能选择本域角色；owner 的变更请使用域所有权转让。
            </DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault()
              void save()
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="domain-member-username">用户名</Label>
              <Input
                id="domain-member-username"
                required
                readOnly={!!editing}
                value={username}
                onChange={(event) => setUsername(event.target.value)}
              />
            </div>
            <label className="block space-y-2 text-sm">
              域角色
              <Select value={role} onValueChange={(value) => setRole(value)}>
                <SelectTrigger aria-label="成员域角色" className="h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {remote.data?.roles.map((item) => (
                    <SelectItem
                      key={item.key}
                      value={item.key}
                      disabled={item.permissions.some((permission) => !can(permission))}
                    >
                      {item.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
            <label className="block space-y-2 text-sm">
              成员状态
              <Select value={status} onValueChange={(value) => setStatus(value as typeof status)}>
                <SelectTrigger aria-label="域成员状态" className="h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(statuses).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
            <Button type="submit" loading={busy}>
              保存成员
            </Button>
          </form>
        </DialogContent>
      </Dialog>
    </section>
  )
}
