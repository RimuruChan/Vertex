import { useCallback, useState } from 'react'
import type { DomainPermission, DtoRoleResponse } from '@/generated/api/model'
import { getApiDomainPermissions } from '@/generated/api/vertex'
import { useDomain } from '@/domain/DomainContext'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, Skeleton } from '@/components/ui/misc'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { useToast } from '@/components/ui/toast'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { apiError } from '@/lib/format'

export default function DomainRolesPage() {
  const active = useActiveRef()
  const { can, refresh } = useDomain(),
    api = useDomainAPI(),
    toast = useToast(),
    confirm = useConfirm()
  const [open, setOpen] = useState(false),
    [editing, setEditing] = useState<DtoRoleResponse | null>(null),
    [key, setKey] = useState(''),
    [name, setName] = useState(''),
    [permissions, setPermissions] = useState<DomainPermission[]>([]),
    [busy, setBusy] = useState(false)
  const load = useCallback(
    async (signal: AbortSignal) => {
      const [roles, catalog] = await Promise.all([
        api.getApiRoles({ signal }),
        getApiDomainPermissions({ signal }),
      ])
      return { roles: roles.items, catalog: catalog.items }
    },
    [api],
  )
  const remote = useRemote(load)
  function edit(role: DtoRoleResponse | null) {
    setEditing(role)
    setKey(role?.key ?? '')
    setName(role?.name ?? '')
    setPermissions(role?.permissions ?? [])
    setOpen(true)
  }
  async function save() {
    if (busy || !can('domain.roles.manage')) return
    setBusy(true)
    try {
      await api.putApiRolesRole(key.trim(), { name, permissions })
      setOpen(false)
      remote.reload()
      await refresh()
      toast.success('角色已保存')
    } catch (error) {
      toast.error(apiError(error, '保存失败'))
    } finally {
      setBusy(false)
    }
  }
  async function remove(role: DtoRoleResponse) {
    if (
      busy ||
      !(await confirm({
        title: `删除角色「${role.name}」？`,
        description: '仍有成员使用的角色不能删除，请先调整这些成员的角色。',
        confirmLabel: '删除角色',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await api.deleteApiRolesRole(role.key)
      remote.reload()
      toast.success('角色已删除')
    } catch (error) {
      toast.error(apiError(error, '删除失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          角色是域权限集合，不等于站点管理员或题目 owner。
        </p>
        {can('domain.roles.manage') && <Button onClick={() => edit(null)}>新建角色</Button>}
      </div>
      {remote.error ? (
        <EmptyState
          title="角色加载失败"
          description={remote.error}
          action={<Button onClick={remote.reload}>重试</Button>}
        />
      ) : !remote.data ? (
        <Skeleton className="h-56" />
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {remote.data.roles.map((role) => (
            <article key={role.key} className="surface-panel space-y-3 p-5">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h2 className="font-medium">{role.name}</h2>
                <span className="text-xs text-muted-foreground">
                  {role.key}
                  {role.builtin ? ' · 内置' : ''}
                </span>
              </div>
              <ul className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                {role.permissions.map((permission) => (
                  <li className="rounded-md bg-muted px-2 py-1" key={permission}>
                    {remote.data!.catalog.find((item) => item.key === permission)?.label ??
                      permission}
                  </li>
                ))}
                {!role.permissions.length && <li>没有额外域能力；已有资源所有权独立计算。</li>}
              </ul>
              {can('domain.roles.manage') && !role.builtin && (
                <div className="flex gap-2">
                  <Button size="sm" variant="outline" onClick={() => edit(role)}>
                    编辑角色
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    onClick={() => void remove(role)}
                  >
                    删除角色
                  </Button>
                </div>
              )}
            </article>
          ))}
        </div>
      )}
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) setOpen(value)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? '编辑域角色' : '新建域角色'}</DialogTitle>
            <DialogDescription>只能授予自己拥有的域能力，不能授予站点角色。</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault()
              void save()
            }}
          >
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="domain-role-key">角色标识</Label>
                <Input
                  id="domain-role-key"
                  required
                  readOnly={!!editing}
                  pattern="[a-z][a-z0-9_-]{0,31}"
                  maxLength={32}
                  value={key}
                  onChange={(event) => setKey(event.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="domain-role-name">显示名称</Label>
                <Input
                  id="domain-role-name"
                  required
                  maxLength={100}
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                />
              </div>
            </div>
            <fieldset className="grid max-h-72 gap-3 overflow-y-auto sm:grid-cols-2">
              <legend className="mb-3 text-sm font-medium">权限</legend>
              {remote.data?.catalog.map((item) => (
                <label key={item.key} className="flex items-start gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="mt-1"
                    checked={permissions.includes(item.key)}
                    disabled={!can(item.key)}
                    onChange={(event) =>
                      setPermissions(
                        event.target.checked
                          ? [...permissions, item.key]
                          : permissions.filter((key) => key !== item.key),
                      )
                    }
                  />
                  {item.label}
                </label>
              ))}
            </fieldset>
            <Button type="submit" loading={busy}>
              保存角色
            </Button>
          </form>
        </DialogContent>
      </Dialog>
    </section>
  )
}
