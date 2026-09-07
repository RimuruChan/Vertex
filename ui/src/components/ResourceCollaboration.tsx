import { useCallback, useId, useMemo, useState } from 'react'
import type {
  DtoProblemGrantResponse,
  DtoContestGrantRequest,
  DtoProblemGrantRequest,
  DtoSetAccessRequest,
} from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { Link } from '@/domain/navigation'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'

type Grant = Omit<DtoProblemGrantResponse, 'role'> & { role: string }
type GrantInput = { username?: string; group?: string; role: string }
const labels: Record<string, string> = {
  reader: '只读审阅',
  editor: '编辑协作',
  jury: '裁判',
  observer: '赛务观察员',
  participant: '参赛资格',
}

export default function ResourceCollaboration({
  kind,
  id,
  ownerId,
  ownerName,
  manage,
  transfer,
  onChanged,
}: {
  kind: 'problem' | 'contest' | 'set'
  id: string
  ownerId: string
  ownerName: string
  manage: boolean
  transfer: boolean
  onChanged: () => void
}) {
  const api = useDomainAPI(),
    active = useActiveRef(),
    confirm = useConfirm(),
    toast = useToast(),
    prefix = useId()
  const roles =
    kind === 'contest' ? ['editor', 'jury', 'observer', 'participant'] : ['reader', 'editor']
  const title = kind === 'problem' ? '题目' : kind === 'contest' ? '比赛' : '题单'
  const [subject, setSubject] = useState<'user' | 'group'>('user'),
    [target, setTarget] = useState(''),
    [role, setRole] = useState(roles[0]),
    [owner, setOwner] = useState(''),
    [busy, setBusy] = useState(false)
  const client = useMemo(() => {
    if (kind === 'problem')
      return {
        list: (signal: AbortSignal) => api.getApiAdminProblemsIdAccess(id, { signal }),
        save: (input: GrantInput) =>
          api.putApiAdminProblemsIdAccess(id, input as DtoProblemGrantRequest),
        remove: (grant: number) => api.deleteApiAdminProblemsIdAccessGrant(id, grant),
        transfer: (username: string) => api.putApiAdminProblemsIdOwner(id, { username }),
      }
    if (kind === 'contest')
      return {
        list: (signal: AbortSignal) => api.getApiContestsIdAccess(id, { signal }),
        save: (input: GrantInput) =>
          api.putApiContestsIdAccess(id, input as DtoContestGrantRequest),
        remove: (grant: number) => api.deleteApiContestsIdAccessGrant(id, grant),
        transfer: (username: string) => api.putApiContestsIdOwner(id, { username }),
      }
    return {
      list: (signal: AbortSignal) => api.getApiProblemSetsIdAccess(id, { signal }),
      save: (input: GrantInput) => api.putApiProblemSetsIdAccess(id, input as DtoSetAccessRequest),
      remove: (grant: number) => api.deleteApiProblemSetsIdAccessGrantId(id, grant),
      transfer: (username: string) => api.putApiProblemSetsIdOwner(id, { username }),
    }
  }, [api, id, kind])
  const load = useCallback(
    async (signal: AbortSignal): Promise<Grant[]> => {
      const result = await client.list(signal)
      return result.items
    },
    [client, ownerId],
  )
  const remote = useRemote(load)
  async function save() {
    if (busy || !manage || !target.trim()) return
    setBusy(true)
    try {
      await client.save({
        role,
        ...(subject === 'user' ? { username: target.trim() } : { group: target.trim() }),
      })
      if (!active.current) return
      setTarget('')
      remote.reload()
      onChanged()
      toast.success('协作授权已保存')
    } catch (error) {
      if (active.current) toast.error(apiError(error, '授权失败'))
    } finally {
      setBusy(false)
    }
  }
  async function remove(grant: Grant) {
    if (
      busy ||
      !manage ||
      !(await confirm({
        title: `移除 ${grant.username ?? grant.groupName} 的${labels[grant.role] ?? grant.role}授权？`,
        description: '只移除这一项授权。其他直接授权、group 继承或域角色仍可能提供访问权。',
        confirmLabel: '移除授权',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await client.remove(grant.id)
      if (!active.current) return
      remote.reload()
      onChanged()
      toast.success('该项授权已移除')
    } catch (error) {
      if (active.current) toast.error(apiError(error, '移除失败'))
    } finally {
      setBusy(false)
    }
  }
  async function changeOwner() {
    if (
      busy ||
      !transfer ||
      !owner.trim() ||
      !(await confirm({
        title: `将${title}转让给 ${owner.trim()}？`,
        description:
          '对方必须是当前域的有效成员。创建记录会保留，但旧 owner 不会因曾创建资源而保留管理权；已有 group 或其他授权单独生效。',
        confirmLabel: '确认转让',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await client.transfer(owner.trim())
      if (!active.current) return
      setOwner('')
      onChanged()
      remote.reload()
      toast.success('所有权已转让')
    } catch (error) {
      if (active.current) toast.error(apiError(error, '转让失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card className="space-y-4 p-4 sm:p-5">
      <div>
        <h2 className="font-medium">协作与所有权</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          Owner：{ownerName || '当前资源所有者'}。
          {kind === 'set'
            ? '题单协作不授予其中题目的访问权。'
            : kind === 'contest'
              ? '编辑、赛务和参赛资格是独立角色，可以组合授权。'
              : '只读协作者可审阅题目包；编辑协作者可修改材料和构建，发布与权限管理属于 owner/域资源管理者。'}
        </p>
      </div>
      {remote.error ? (
        <div className="flex items-center gap-3">
          <p role="alert" className="text-sm text-destructive">
            {remote.error}
          </p>
          <Button size="sm" variant="outline" onClick={remote.reload}>
            重试
          </Button>
        </div>
      ) : !remote.data ? (
        <p role="status" className="text-sm text-muted-foreground">
          正在加载协作者…
        </p>
      ) : remote.data.length ? (
        <ul className="divide-y">
          {remote.data.map((grant) => (
            <li className="flex flex-wrap items-center gap-3 py-3 text-sm" key={grant.id}>
              <span className="min-w-0 flex-1 break-words">
                {grant.username ?? grant.groupName}
                <span className="ml-2 text-xs text-muted-foreground">
                  {grant.groupId ? 'group 继承来源' : '用户直接授权'}
                </span>
              </span>
              <span>{labels[grant.role] ?? grant.role}</span>
              {manage && (
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy}
                  aria-label={`移除 ${grant.username ?? grant.groupName} 的${labels[grant.role] ?? grant.role}授权`}
                  onClick={() => void remove(grant)}
                >
                  移除
                </Button>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">
          暂无显式协作授权。Owner 和域资源管理者的权限不列在这里。
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        Group 授权按当前有效组成员计算，移除用户的直接授权不会抵消其 group 继承。
        <Link className="ml-1 text-primary" to="/groups">
          查看本域群组
        </Link>
      </p>
      {manage && (
        <form
          className="grid gap-3 border-t pt-4 sm:grid-cols-[auto_minmax(0,1fr)_auto_auto] sm:items-end"
          onSubmit={(event) => {
            event.preventDefault()
            void save()
          }}
        >
          <div className="space-y-2">
            <Label htmlFor={`${prefix}-subject`}>授权对象</Label>
            <select
              id={`${prefix}-subject`}
              className="h-9 w-full rounded-md border bg-background px-2 text-sm"
              value={subject}
              onChange={(event) => {
                setSubject(event.target.value as typeof subject)
                setTarget('')
              }}
            >
              <option value="user">用户</option>
              <option value="group">Group</option>
            </select>
          </div>
          <div className="min-w-0 space-y-2">
            <Label htmlFor={`${prefix}-target`}>
              {subject === 'user' ? '用户名' : '域内群组编号'}
            </Label>
            <Input
              id={`${prefix}-target`}
              required
              value={target}
              onChange={(event) => setTarget(event.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor={`${prefix}-role`}>协作角色</Label>
            <select
              id={`${prefix}-role`}
              className="h-9 w-full rounded-md border bg-background px-2 text-sm"
              value={role}
              onChange={(event) => setRole(event.target.value)}
            >
              {roles.map((role) => (
                <option key={role} value={role}>
                  {labels[role]}
                </option>
              ))}
            </select>
          </div>
          <Button type="submit" loading={busy}>
            保存授权
          </Button>
        </form>
      )}
      {transfer && (
        <details className="border-t pt-3">
          <summary className="cursor-pointer text-sm text-muted-foreground">转让所有权</summary>
          <form
            className="mt-3 flex flex-wrap gap-2"
            onSubmit={(event) => {
              event.preventDefault()
              void changeOwner()
            }}
          >
            <Input
              className="max-w-xs"
              aria-label="新 owner 的用户名"
              required
              value={owner}
              onChange={(event) => setOwner(event.target.value)}
            />
            <Button type="submit" variant="outline" disabled={busy}>
              转让{title}
            </Button>
          </form>
        </details>
      )}
    </Card>
  )
}
