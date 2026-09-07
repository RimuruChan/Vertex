import { useEffect, useRef, useState } from 'react'
import { useAuth } from '@/auth/AuthContext'
import type { DtoSetAccessResponse, DtoSetResponse } from '@/generated/api/model'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'

export default function SetCollaboration({
  set,
  onTransferred,
}: {
  set: DtoSetResponse
  onTransferred: () => void
}) {
  const {
    getApiProblemSetsIdAccess,
    putApiProblemSetsIdAccess,
    deleteApiProblemSetsIdAccessGrantId,
    putApiProblemSetsIdOwner,
  } = useDomainAPI()
  const { user } = useAuth()
  const confirm = useConfirm(),
    toast = useToast()
  const [grants, setGrants] = useState<DtoSetAccessResponse[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [reload, setReload] = useState(0)
  const [busy, setBusy] = useState(false)
  const [kind, setKind] = useState<'user' | 'group'>('user')
  const [target, setTarget] = useState('')
  const [role, setRole] = useState<'reader' | 'editor'>('reader')
  const [owner, setOwner] = useState('')
  const active = useRef(true)
  useEffect(() => {
    active.current = true
    return () => {
      active.current = false
    }
  }, [])
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError(null)
    getApiProblemSetsIdAccess(set.id, { signal: controller.signal })
      .then((result) => {
        if (!controller.signal.aborted) setGrants(result.items)
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(apiError(cause, '协作权限加载失败'))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [set.id, set.ownerId, user?.id, reload])
  async function grant() {
    if (busy || !target.trim() || !set.permissions.manageAccess) return
    setBusy(true)
    try {
      await putApiProblemSetsIdAccess(set.id, {
        role,
        ...(kind === 'user' ? { username: target.trim() } : { group: target.trim() }),
      })
      if (!active.current) return
      setTarget('')
      setReload((v) => v + 1)
      toast.success('协作权限已更新')
    } catch (cause) {
      if (active.current) toast.error(apiError(cause, '授权失败'))
    } finally {
      setBusy(false)
    }
  }
  async function remove(grant: DtoSetAccessResponse) {
    if (busy || !set.permissions.manageAccess) return
    if (
      !(await confirm({
        title: '移除此项授权？',
        description: '只移除这项直接授权；其他用户、群组或域角色提供的权限仍然有效。',
        confirmLabel: '移除授权',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await deleteApiProblemSetsIdAccessGrantId(set.id, grant.id)
      if (!active.current) return
      setReload((v) => v + 1)
      toast.success('授权已移除')
    } catch (cause) {
      if (active.current) toast.error(apiError(cause, '移除失败'))
    } finally {
      setBusy(false)
    }
  }
  async function transfer() {
    if (busy || !owner.trim() || !set.permissions.transfer) return
    if (
      !(await confirm({
        title: `将题单转让给 ${owner.trim()}？`,
        description:
          '对方必须是当前域的有效成员。创建记录会保留，但你不会因为曾创建题单而继续拥有管理权。题目本身的权限不变。',
        confirmLabel: '确认转让',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await putApiProblemSetsIdOwner(set.id, { username: owner.trim() })
      if (!active.current) return
      toast.success('所有权已转让')
      onTransferred()
    } catch (cause) {
      if (active.current) toast.error(apiError(cause, '转让失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card className="p-4 sm:p-5">
      <details>
        <summary className="cursor-pointer text-sm font-medium">协作与所有权</summary>
        <div className="mt-4 flex flex-col gap-4 text-sm">
          <p className="text-muted-foreground">
            Owner：{set.ownerName}。协作只作用于本题单，不会授予其中题目的访问权限。
          </p>
          {loading ? (
            <p role="status">正在加载协作者…</p>
          ) : error ? (
            <div className="flex flex-wrap items-center gap-2">
              <p role="alert" className="text-destructive">
                {error}
              </p>
              <Button variant="outline" size="sm" onClick={() => setReload((v) => v + 1)}>
                重试
              </Button>
            </div>
          ) : grants.length ? (
            <ul className="divide-y divide-border">
              {grants.map((grant) => (
                <li key={grant.id} className="flex flex-wrap items-center gap-3 py-2">
                  <span className="min-w-0 flex-1 break-all">
                    {grant.username ?? grant.groupName}{' '}
                    <span className="text-muted-foreground">
                      {grant.groupId ? '· 群组' : '· 用户'}
                    </span>
                  </span>
                  <span className="text-muted-foreground">
                    {grant.role === 'editor' ? '编辑' : '只读'}
                  </span>
                  {set.permissions.manageAccess && (
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy}
                      onClick={() => remove(grant)}
                      aria-label={`移除 ${grant.username ?? grant.groupName} 的授权`}
                    >
                      移除
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-muted-foreground">
              暂无直接协作者。域资源管理者的权限不会列在这里。
            </p>
          )}
          {set.permissions.manageAccess && (
            <form
              className="grid gap-3 border-t border-border pt-4 sm:grid-cols-[auto_minmax(0,1fr)_auto_auto] sm:items-end"
              onSubmit={(event) => {
                event.preventDefault()
                void grant()
              }}
            >
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="set-grant-kind">授权对象</Label>
                <select
                  id="set-grant-kind"
                  className="h-9 rounded-md border border-input bg-background px-2"
                  value={kind}
                  onChange={(e) => setKind(e.target.value as 'user' | 'group')}
                >
                  <option value="user">用户</option>
                  <option value="group">群组</option>
                </select>
              </div>
              <div className="flex min-w-0 flex-col gap-1.5">
                <Label htmlFor="set-grant-target">
                  {kind === 'user' ? '用户名' : '域内群组编号'}
                </Label>
                <Input
                  id="set-grant-target"
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  autoComplete="off"
                />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="set-grant-role">角色</Label>
                <select
                  id="set-grant-role"
                  className="h-9 rounded-md border border-input bg-background px-2"
                  value={role}
                  onChange={(e) => setRole(e.target.value as 'reader' | 'editor')}
                >
                  <option value="reader">只读</option>
                  <option value="editor">编辑</option>
                </select>
              </div>
              <Button type="submit" loading={busy} disabled={!target.trim()}>
                保存授权
              </Button>
            </form>
          )}
          {set.permissions.transfer && (
            <details className="border-t border-border pt-4">
              <summary className="cursor-pointer text-muted-foreground">转让所有权</summary>
              <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-end">
                <div className="flex min-w-0 flex-1 flex-col gap-1.5">
                  <Label htmlFor="set-new-owner">新 owner 的用户名</Label>
                  <Input
                    id="set-new-owner"
                    value={owner}
                    onChange={(e) => setOwner(e.target.value)}
                    autoComplete="off"
                  />
                </div>
                <Button variant="outline" disabled={busy || !owner.trim()} onClick={transfer}>
                  转让题单
                </Button>
              </div>
            </details>
          )}
        </div>
      </details>
    </Card>
  )
}
