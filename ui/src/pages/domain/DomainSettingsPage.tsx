import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useState } from 'react'
import { useDomain } from '@/domain/DomainContext'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useActiveRef } from '@/domain/useActiveRef'
import { Button } from '@/components/ui/button'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useToast } from '@/components/ui/toast'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { apiError } from '@/lib/format'

export default function DomainSettingsPage() {
  const active = useActiveRef()
  const { domain, can, refresh } = useDomain(),
    api = useDomainAPI(),
    toast = useToast(),
    confirm = useConfirm()
  const [name, setName] = useState(domain.name),
    [description, setDescription] = useState(domain.description),
    [visibility, setVisibility] = useState(domain.visibility),
    [joinPolicy, setJoinPolicy] = useState(domain.joinPolicy),
    [owner, setOwner] = useState(''),
    [busy, setBusy] = useState(false)
  async function save() {
    if (busy || !can('domain.settings.manage')) return
    setBusy(true)
    try {
      await api.putApi({ name, description, visibility, joinPolicy })
      await refresh()
      toast.success('域设置已保存')
    } catch (error) {
      toast.error(apiError(error, '保存失败'))
    } finally {
      setBusy(false)
    }
  }
  async function archive() {
    if (busy || !domain.canArchive) return
    const next = !domain.archived
    if (
      !(await confirm({
        title: next ? '归档这个域？' : '恢复这个域？',
        description: next
          ? '资源和成员会保留，域内写操作停止。之后仍可由所有者或站点维护者恢复。'
          : '恢复后，各资源按原有权限继续使用。',
        confirmLabel: next ? '归档' : '恢复',
        destructive: next,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await api.putApiArchive({ archived: next })
      await refresh()
      toast.success(next ? '域已归档' : '域已恢复')
    } catch (error) {
      toast.error(apiError(error, '操作失败'))
    } finally {
      setBusy(false)
    }
  }
  async function transfer() {
    if (busy || !domain.canTransfer || !owner.trim()) return
    if (
      !(await confirm({
        title: `将域所有权转给 ${owner.trim()}？`,
        description: '目标必须是有效域成员。此操作不会转让域内题目、比赛或群组的所有权。',
        confirmLabel: '转让所有权',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    try {
      await api.putApiOwner({ username: owner.trim() })
      setOwner('')
      await refresh()
      toast.success('域所有权已转让')
    } catch (error) {
      toast.error(apiError(error, '转让失败'))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="space-y-5">
      <form
        className="surface-panel space-y-4 p-5"
        onSubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <div>
          <h2 className="font-medium">空间信息</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            固定标识：{domain.slug} · 所有者：{domain.ownerName || '站点维护'}
            {domain.archived ? ' · 已归档' : ''}
          </p>
        </div>
        <fieldset
          disabled={!can('domain.settings.manage') || busy}
          className="space-y-4 disabled:opacity-70"
        >
          <div className="space-y-2">
            <Label htmlFor="domain-setting-name">显示名称</Label>
            <Input
              id="domain-setting-name"
              required
              maxLength={100}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="domain-setting-description">介绍</Label>
            <Textarea
              id="domain-setting-description"
              maxLength={8000}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="space-y-2 text-sm">
              可见性
              <Select
                disabled={domain.official}

                value={visibility}
                onValueChange={(value) => setVisibility(value as typeof visibility)}
              >
                <SelectTrigger aria-label="域设置可见性" className="h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="public">公开</SelectItem>
                  <SelectItem value="private">私有</SelectItem>
                </SelectContent>
              </Select>
            </label>
            <label className="space-y-2 text-sm">
              加入方式
              <Select
                disabled={domain.official}

                value={joinPolicy}
                onValueChange={(value) => setJoinPolicy(value as typeof joinPolicy)}
              >
                <SelectTrigger aria-label="域设置加入方式" className="h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="open">开放加入</SelectItem>
                  <SelectItem value="approval">申请审核</SelectItem>
                  <SelectItem value="invite">邀请加入</SelectItem>
                </SelectContent>
              </Select>
            </label>
          </div>
          {can('domain.settings.manage') && (
            <Button type="submit" loading={busy}>
              保存域设置
            </Button>
          )}
        </fieldset>
      </form>
      {(domain.canArchive || domain.canTransfer) && (
        <section className="surface-panel space-y-5 border-destructive/25 p-5">
          <div>
            <h2 className="font-medium">所有权与归档</h2>
            <p className="mt-1 text-sm text-muted-foreground">只影响当前域，不删除数据。</p>
          </div>
          {domain.canTransfer && (
            <form
              className="flex flex-wrap gap-2"
              onSubmit={(event) => {
                event.preventDefault()
                void transfer()
              }}
            >
              <Input
                className="max-w-xs"
                aria-label="新域所有者用户名"
                placeholder="有效域成员用户名"
                value={owner}
                onChange={(event) => setOwner(event.target.value)}
                required
              />
              <Button type="submit" variant="outline" disabled={busy}>
                转让域
              </Button>
            </form>
          )}
          {domain.canArchive && (
            <Button variant="outline" disabled={busy} onClick={() => void archive()}>
              {domain.archived ? '恢复域' : '归档域'}
            </Button>
          )}
        </section>
      )}
    </div>
  )
}
