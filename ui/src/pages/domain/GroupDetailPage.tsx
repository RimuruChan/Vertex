import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import type { DtoGroupResponse, DtoGroupMemberResponse } from '@/generated/api/model'
import { Link, useNavigate } from '@/domain/navigation'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { useCanonicalResourcePath } from '@/hooks/useCanonicalPath'
import { Button } from '@/components/ui/button'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, PageSpinner } from '@/components/ui/misc'
import { Pagination } from '@/components/ui/pagination'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'

export default function GroupDetailPage() {
  const active = useActiveRef()
  const { group: ref = '' } = useParams(),
    api = useDomainAPI(),
    navigate = useNavigate(),
    toast = useToast(),
    confirm = useConfirm()
  const [page, setPage] = useState(1),
    [query, setQuery] = useState(''),
    [keyword, setKeyword] = useState(''),
    [open, setOpen] = useState(false),
    [memberName, setMemberName] = useState(''),
    [memberRole, setMemberRole] = useState<'member' | 'manager'>('member'),
    [editing, setEditing] = useState(false),
    [newOwner, setNewOwner] = useState(''),
    [busy, setBusy] = useState(false)
  const load = useCallback(
    async (signal: AbortSignal) => {
      const group = await api.getApiGroupsGroup(ref, { signal })
      const members =
        group.canManage || group.viewerRole
          ? await api.getApiGroupsGroupMembers(ref, { page, size: 20, keyword }, { signal })
          : null
      return { group, members }
    },
    [api, ref, page, keyword],
  )
  const remote = useRemote(load)
  useCanonicalResourcePath('groups', ref, remote.data?.group)
  if (remote.error)
    return (
      <div className="page-shell">
        <EmptyState
          title="无法打开群组"
          description={remote.error}
          action={<Button onClick={remote.reload}>重试</Button>}
        />
      </div>
    )
  if (!remote.data) return <PageSpinner />
  const { group, members } = remote.data
  function edit(member?: DtoGroupMemberResponse) {
    setMemberName(member?.username ?? '')
    setMemberRole((member?.role as typeof memberRole) ?? 'member')
    setEditing(!!member)
    setOpen(true)
  }
  async function saveMember() {
    if (busy || !group.canManage) return
    setBusy(true)
    try {
      await api.putApiGroupsGroupMembersUsername(ref, memberName.trim(), { role: memberRole })
      setOpen(false)
      remote.reload()
      toast.success('组成员已保存')
    } catch (error) {
      toast.error(apiError(error, '成员更新失败'))
    } finally {
      setBusy(false)
    }
  }
  async function removeMember(member: DtoGroupMemberResponse) {
    if (
      busy ||
      !group.canManage ||
      !(await confirm({
        title: `将 ${member.username} 移出群组？`,
        description:
          '此操作只移除本组成员关系，不会移出域，也不会删除账号。由本组继承的资源授权随之失效。',
        confirmLabel: '移出群组',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await api.deleteApiGroupsGroupMembersUsername(ref, member.username)
      remote.reload()
      toast.success('已移出群组')
    } catch (error) {
      toast.error(apiError(error, '移除失败'))
    } finally {
      setBusy(false)
    }
  }
  async function transfer() {
    if (
      busy ||
      !group.canTransfer ||
      !newOwner.trim() ||
      !(await confirm({
        title: `将群组转给 ${newOwner.trim()}？`,
        description: '目标必须是本域有效成员。旧所有者的普通组内关系不会自动删除。',
        confirmLabel: '转让群组',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await api.putApiGroupsGroupOwner(ref, { username: newOwner.trim() })
      setNewOwner('')
      remote.reload()
      toast.success('群组已转让')
    } catch (error) {
      toast.error(apiError(error, '转让失败'))
    } finally {
      setBusy(false)
    }
  }
  async function remove() {
    if (
      busy ||
      !group.canDelete ||
      !(await confirm({
        title: `删除群组「${group.name}」？`,
        description: '群组及其成员关系、组授权会移除，题目、比赛等资源不会删除。无法撤销。',
        confirmLabel: '删除群组',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await api.deleteApiGroupsGroup(ref)
      if (!active.current) return
      navigate('/groups')
      toast.success('群组已删除')
    } catch (error) {
      toast.error(apiError(error, '删除失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="page-shell flex flex-col gap-5">
      <Link className="w-fit text-sm text-muted-foreground hover:text-primary" to="/groups">
        ← 返回群组
      </Link>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs text-muted-foreground">群组 #{group.publicId}</p>
          <h1 className="mt-1 text-2xl font-semibold">{group.name}</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            owner：{group.ownerName} · {group.memberCount} 位有效成员
          </p>
        </div>
        <Button variant="outline" onClick={remote.reload} disabled={remote.loading}>
          刷新
        </Button>
      </div>
      <GroupSettings key={group.id} group={group} onSaved={remote.reload} />
      <section className="surface-panel space-y-4 p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="font-medium">组成员</h2>
          {group.canManage && <Button onClick={() => edit()}>添加组成员</Button>}
        </div>
        {members ? (
          <>
            <form
              className="flex max-w-lg gap-2"
              onSubmit={(event) => {
                event.preventDefault()
                setPage(1)
                setKeyword(query.trim())
              }}
            >
              <Input
                aria-label="搜索组成员"
                placeholder="按用户名搜索"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
              <Button type="submit" variant="outline">
                搜索
              </Button>
            </form>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="border-b">
                  <tr>
                    <th className="p-3">用户名</th>
                    <th className="p-3">组内身份</th>
                    <th className="p-3">
                      <span className="sr-only">操作</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {members.items.map((member) => (
                    <tr key={member.userId} className="border-b last:border-0">
                      <td className="p-3">
                        {member.username}
                        {member.userId === group.ownerId ? ' · owner' : ''}
                      </td>
                      <td className="p-3">{member.role === 'manager' ? '组管理者' : '成员'}</td>
                      <td className="p-3 text-right">
                        {group.canManage && member.userId !== group.ownerId && (
                          <div className="flex justify-end gap-2">
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => edit(member)}
                              disabled={busy}
                            >
                              设置
                            </Button>
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => void removeMember(member)}
                              disabled={busy}
                            >
                              移出
                            </Button>
                          </div>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {!members.items.length && <EmptyState title="没有匹配的有效成员" />}
            </div>
            <Pagination page={page} size={20} total={members.total} onChange={setPage} />
          </>
        ) : (
          <p className="text-sm text-muted-foreground">只有组成员与组管理者可以读取成员列表。</p>
        )}
      </section>
      {(group.canTransfer || group.canDelete) && (
        <section className="surface-panel space-y-4 border-destructive/25 p-5">
          <h2 className="font-medium">所有权与删除</h2>
          {group.canTransfer && (
            <form
              className="flex flex-wrap gap-2"
              onSubmit={(event) => {
                event.preventDefault()
                void transfer()
              }}
            >
              <Input
                className="max-w-xs"
                aria-label="新组所有者用户名"
                placeholder="本域有效成员用户名"
                required
                value={newOwner}
                onChange={(event) => setNewOwner(event.target.value)}
              />
              <Button type="submit" variant="outline" disabled={busy}>
                转让群组
              </Button>
            </form>
          )}
          {group.canDelete && (
            <Button variant="destructive" disabled={busy} onClick={() => void remove()}>
              删除群组
            </Button>
          )}
        </section>
      )}
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) setOpen(value)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? '设置组成员' : '添加组成员'}</DialogTitle>
            <DialogDescription>
              组内管理者可以维护本组成员，但不能因此获得域治理权。
            </DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault()
              void saveMember()
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="group-member-name">用户名</Label>
              <Input
                id="group-member-name"
                required
                readOnly={editing}
                value={memberName}
                onChange={(event) => setMemberName(event.target.value)}
              />
            </div>
            <label className="block space-y-2 text-sm">
              组内角色
              <select
                aria-label="组内角色"
                className="h-9 w-full rounded-md border bg-background px-2"
                value={memberRole}
                onChange={(event) => setMemberRole(event.target.value as typeof memberRole)}
              >
                <option value="member">成员</option>
                <option value="manager">组管理者</option>
              </select>
            </label>
            <Button type="submit" loading={busy}>
              保存组成员
            </Button>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function GroupSettings({ group, onSaved }: { group: DtoGroupResponse; onSaved: () => void }) {
  const api = useDomainAPI(),
    toast = useToast()
  const [name, setName] = useState(group.name),
    [description, setDescription] = useState(group.description),
    [busy, setBusy] = useState(false)
  const baseline = useRef({ name: group.name, description: group.description })
  useEffect(() => {
    const previous = baseline.current
    setName((current) => (!group.canManage || current === previous.name ? group.name : current))
    setDescription((current) =>
      !group.canManage || current === previous.description ? group.description : current,
    )
    baseline.current = { name: group.name, description: group.description }
  }, [group.name, group.description, group.canManage])
  async function save() {
    if (busy || !group.canManage) return
    setBusy(true)
    try {
      await api.putApiGroupsGroup(group.id, { name, description })
      onSaved()
      toast.success('组设置已保存')
    } catch (error) {
      toast.error(apiError(error, '保存失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <form
      className="surface-panel space-y-4 p-5"
      onSubmit={(event) => {
        event.preventDefault()
        void save()
      }}
    >
      <h2 className="font-medium">组设置</h2>
      <fieldset disabled={!group.canManage || busy} className="space-y-4 disabled:opacity-70">
        <div className="space-y-2">
          <Label htmlFor="group-setting-name">组名称</Label>
          <Input
            id="group-setting-name"
            required
            maxLength={100}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="group-setting-description">组介绍</Label>
          <Textarea
            id="group-setting-description"
            maxLength={8000}
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </div>
        {group.canManage && (
          <Button type="submit" loading={busy}>
            保存组设置
          </Button>
        )}
      </fieldset>
    </form>
  )
}
