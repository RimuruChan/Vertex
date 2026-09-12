import { useCallback, useId, useMemo, useState, type ReactNode } from 'react'
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
import { ChoiceSelect } from '@/components/ui/choice-select'
import { Users, UserRound, ShieldCheck, Plus, Trash2 } from 'lucide-react'
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
  children,
  participantsOnly = false,
}: {
  kind: 'problem' | 'contest' | 'set'
  id: string
  ownerId: string
  ownerName: string
  manage: boolean
  transfer: boolean
  onChanged: () => void
  participantsOnly?: boolean
  children?: ReactNode
}) {
  const api = useDomainAPI(),
    active = useActiveRef(),
    confirm = useConfirm(),
    toast = useToast(),
    prefix = useId()
  const roles =
    kind === 'contest'
      ? participantsOnly
        ? ['participant']
        : ['editor', 'jury', 'observer']
      : ['reader', 'editor']
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
      return kind === 'contest'
        ? result.items.filter((grant) =>
            participantsOnly ? grant.role === 'participant' : grant.role !== 'participant',
          )
        : result.items
    },
    [client, ownerId, kind, participantsOnly],
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
  const roleDescription: Record<string, string> = {
    reader: '查看材料，不修改内容。',
    editor: '编辑内容与编排，不自动获得权限管理能力。',
    jury: '查看赛务数据、回复答疑并重新评测。',
    observer: '查看赛务数据，不进行提交或管理操作。',
    participant: '授予参赛资格，用户仍需完成报名。',
  }
  if (participantsOnly)
    return (
      <div className="space-y-3 border-y border-border/60 py-4">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h3 className="text-sm font-medium">指定参赛用户与群组</h3>
          <span className="text-xs text-muted-foreground">授权单独保存，立即生效</span>
        </div>
        {manage && (
          <div className="flex flex-wrap gap-2">
            <div className="w-24 shrink-0">
              <ChoiceSelect
                label="参赛对象类型"
                value={subject}
                onValueChange={(value) => {
                  setSubject(value as typeof subject)
                  setTarget('')
                }}
                disabled={busy}
                options={[
                  ['user', '用户'],
                  ['group', '群组'],
                ]}
              />
            </div>
            <Input
              className="min-w-32 flex-1"
              aria-label={subject === 'user' ? '参赛用户名' : '参赛群组编号'}
              placeholder={subject === 'user' ? '输入用户名' : '输入群组编号'}
              value={target}
              disabled={busy}
              onChange={(event) => setTarget(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  void save()
                }
              }}
            />
            <Button
              type="button"
              variant="outline"
              disabled={busy || !target.trim()}
              loading={busy}
              onClick={() => void save()}
            >
              <Plus />
              添加
            </Button>
          </div>
        )}
        {remote.error ? (
          <div role="alert" className="flex items-center gap-2 text-sm text-destructive">
            {remote.error}
            <Button type="button" variant="outline" size="sm" onClick={remote.reload}>
              重试
            </Button>
          </div>
        ) : !remote.data ? (
          <p role="status" className="text-xs text-muted-foreground">
            正在加载名单…
          </p>
        ) : remote.data.length ? (
          <ul className="divide-y divide-border/50">
            {remote.data.map((grant) => (
              <li key={grant.id} className="flex items-center gap-2 py-2 text-sm">
                {grant.groupId ? (
                  <Users className="size-4 text-muted-foreground" />
                ) : (
                  <UserRound className="size-4 text-muted-foreground" />
                )}
                <span className="min-w-0 flex-1 break-words">
                  {grant.username ?? grant.groupName}
                </span>
                <span className="text-xs text-muted-foreground">
                  {grant.groupId ? '群组' : '用户'}
                </span>
                {manage && (
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    disabled={busy}
                    aria-label={`移除 ${grant.username ?? grant.groupName} 的参赛资格`}
                    onClick={() => void remove(grant)}
                  >
                    <Trash2 />
                  </Button>
                )}
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-xs text-muted-foreground">
            尚未指定参赛对象。添加后，符合资格的成员仍需报名。
          </p>
        )}
      </div>
    )
  const GrantForm = 'form'
  return (
    <div className="flex flex-col gap-5">
      {!participantsOnly && (
        <div>
          <h2 className="text-lg font-semibold">人员与权限</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            按用户或群组分配职责。授权保存后立即生效。
          </p>
        </div>
      )}
      {!participantsOnly && (
        <Card className="flex items-center gap-4 rounded-xl p-5">
          <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/10 text-primary">
            <ShieldCheck className="size-5" />
          </span>
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground">当前负责人</p>
            <p className="mt-1 break-words text-sm font-semibold">
              {ownerName || '当前资源所有者'}
            </p>
          </div>
          <span className="ml-auto rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
            管理权限
          </span>
        </Card>
      )}
      <Card className="overflow-hidden rounded-xl">
        <div className="flex items-center justify-between border-b border-border px-5 py-4">
          <h3 className="text-sm font-semibold">
            {participantsOnly ? '指定参赛用户与群组' : '当前授权'}
          </h3>
          <span className="text-xs text-muted-foreground">{remote.data?.length ?? 0} 条授权</span>
        </div>
        {remote.error ? (
          <div className="flex items-center justify-between gap-3 p-5">
            <p role="alert" className="text-sm text-destructive">
              {remote.error}
            </p>
            <Button size="sm" variant="outline" onClick={remote.reload}>
              重试
            </Button>
          </div>
        ) : !remote.data ? (
          <p role="status" className="p-8 text-center text-sm text-muted-foreground">
            正在加载协作者…
          </p>
        ) : remote.data.length ? (
          <ul className="divide-y divide-border">
            {remote.data.map((grant) => (
              <li key={grant.id} className="flex flex-wrap items-center gap-3 px-5 py-4">
                <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-muted text-muted-foreground">
                  {grant.groupId ? <Users className="size-4" /> : <UserRound className="size-4" />}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="break-words text-sm font-medium">
                    {grant.username ?? grant.groupName}
                  </p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {grant.groupId ? '群组继承授权' : '用户直接授权'}
                  </p>
                </div>
                <span className="rounded-md bg-primary/8 px-2 py-1 text-xs font-medium text-primary">
                  {labels[grant.role] ?? grant.role}
                </span>
                {manage && (
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                    disabled={busy}
                    aria-label={`移除 ${grant.username ?? grant.groupName} 的${labels[grant.role] ?? grant.role}授权`}
                    onClick={() => void remove(grant)}
                  >
                    <Trash2 />
                  </Button>
                )}
              </li>
            ))}
          </ul>
        ) : (
          <div
            className={
              participantsOnly
                ? 'space-y-1 px-5 py-4'
                : 'flex flex-col items-center gap-2 p-8 text-center'
            }
          >
            {!participantsOnly && <Users className="size-7 text-muted-foreground/50" />}
            <p className="text-sm font-medium">
              {participantsOnly ? '尚未指定参赛用户或群组' : '暂无协作授权'}
            </p>
            <p className="text-xs text-muted-foreground">
              {participantsOnly
                ? '添加指定用户或群组后，符合资格的成员可以报名。'
                : '负责人和域管理员的权限独立生效，不列在这里。'}
            </p>
          </div>
        )}
        <p className="border-t border-border bg-muted/20 px-5 py-3 text-xs leading-relaxed text-muted-foreground">
          群组授权按当前成员计算。移除直接授权不会取消从群组继承的权限。
          <Link className="ml-1 text-primary hover:underline" to="/groups">
            查看本域群组
          </Link>
        </p>
      </Card>
      {manage && (
        <Card
          className={
            participantsOnly
              ? 'space-y-4 rounded-xl p-4'
              : 'grid gap-5 rounded-xl p-5 sm:p-6 lg:grid-cols-[180px_minmax(0,1fr)] lg:gap-8'
          }
        >
          <div>
            <h3 className="text-sm font-semibold">新增授权</h3>
            <p className="mt-2 text-xs leading-relaxed text-muted-foreground">
              {kind === 'contest'
                ? participantsOnly
                  ? '按用户名或群组编号添加，授权单独保存并立即生效；用户仍需报名。'
                  : '编辑与赛务角色可以组合授予。参赛资格在基本设置中管理。'
                : kind === 'set'
                  ? '题单协作不授予其中题目的访问权。'
                  : '按实际职责授予访问或编辑权限。'}
            </p>
          </div>
          <GrantForm
            className="flex min-w-0 flex-col gap-4"
            onKeyDown={(event) => {
              if (
                participantsOnly &&
                event.key === 'Enter' &&
                event.target instanceof HTMLInputElement
              ) {
                event.preventDefault()
                void save()
              }
            }}
            onSubmit={(event) => {
              event.preventDefault()
              void save()
            }}
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor={`${prefix}-subject`}>授权对象</Label>
                <ChoiceSelect
                  id={`${prefix}-subject`}
                  value={subject}
                  disabled={busy}
                  onValueChange={(value) => {
                    setSubject(value as typeof subject)
                    setTarget('')
                  }}
                  options={[
                    ['user', '用户'],
                    ['group', '群组'],
                  ]}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor={`${prefix}-target`}>
                  {subject === 'user' ? '用户名' : '域内群组编号'}
                </Label>
                <Input
                  id={`${prefix}-target`}
                  required={!participantsOnly}
                  disabled={busy}
                  placeholder={subject === 'user' ? '输入用户名' : '输入群组编号'}
                  value={target}
                  onChange={(event) => setTarget(event.target.value)}
                />
              </div>
              {!participantsOnly && (
                <div className="space-y-2">
                  <Label htmlFor={`${prefix}-role`}>协作角色</Label>
                  <ChoiceSelect
                    id={`${prefix}-role`}
                    value={role}
                    disabled={busy}
                    onValueChange={setRole}
                    options={roles.map((role) => [role, labels[role]] as const)}
                  />
                </div>
              )}
              {!participantsOnly && (
                <p className="self-center rounded-lg bg-muted/40 px-3 py-2 text-xs leading-relaxed text-muted-foreground">
                  {roleDescription[role] ?? '根据所选角色授予对应权限。'}
                </p>
              )}
            </div>
            <div className="flex justify-end border-t border-border pt-3">
              <Button
                type={participantsOnly ? 'button' : 'submit'}
                onClick={participantsOnly ? () => void save() : undefined}
                loading={busy}
                disabled={!target.trim()}
              >
                <Plus />
                保存授权
              </Button>
            </div>
          </GrantForm>
        </Card>
      )}
      {children}
      {transfer && (
        <Card className="rounded-xl border-destructive/25 px-5 py-4">
          <details>
            <summary className="cursor-pointer text-sm font-medium">转让负责人</summary>
            <p className="mt-3 text-xs text-muted-foreground">
              转让后，原负责人不再因创建者身份保留管理权限。其他直接授权和群组授权独立生效。
            </p>
            <form
              className="mt-4 flex flex-wrap gap-2"
              onSubmit={(event) => {
                event.preventDefault()
                void changeOwner()
              }}
            >
              <Input
                className="max-w-xs"
                aria-label="新负责人的用户名"
                placeholder="新负责人的用户名"
                required
                disabled={busy}
                value={owner}
                onChange={(event) => setOwner(event.target.value)}
              />
              <Button type="submit" variant="outline" disabled={busy || !owner.trim()}>
                转让{title}
              </Button>
            </form>
          </details>
        </Card>
      )}
    </div>
  )
}
