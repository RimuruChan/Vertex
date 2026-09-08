import { useCallback, useState } from 'react'
import { useParams } from 'react-router-dom'
import type { DtoTagCatalogResponse } from '@/generated/api/model'
import { Link, useNavigate } from '@/domain/navigation'
import { useDomain } from '@/domain/DomainContext'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, PageSpinner } from '@/components/ui/misc'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { useToast } from '@/components/ui/toast'
import { apiError } from '@/lib/format'

export default function TagDetailPage() {
  const { tag = '' } = useParams(),
    api = useDomainAPI()
  const load = useCallback(
    async (signal: AbortSignal) => {
      const [item, catalogue] = await Promise.all([
        api.getApiAdminTagsId(Number(tag), { signal }),
        api.getApiAdminTags({ signal }),
      ])
      return { item, catalogue: catalogue.items }
    },
    [api, tag],
  )
  const remote = useRemote(load)
  if (remote.error)
    return (
      <EmptyState
        title="标签不可用"
        description={remote.error}
        action={<Button onClick={remote.reload}>重试</Button>}
      />
    )
  if (!remote.data) return <PageSpinner />
  return (
    <div className="flex flex-col gap-4">
      <Link className="w-fit text-sm text-muted-foreground hover:text-primary" to="/settings/tags">
        返回标签目录
      </Link>
      <TagEditor
        key={remote.data.item.id}
        item={remote.data.item}
        catalogue={remote.data.catalogue}
      />
    </div>
  )
}

function TagEditor({
  item,
  catalogue,
}: {
  item: DtoTagCatalogResponse
  catalogue: DtoTagCatalogResponse[]
}) {
  const api = useDomainAPI(),
    { can } = useDomain(),
    active = useActiveRef(),
    navigate = useNavigate(),
    confirm = useConfirm(),
    toast = useToast(),
    writable = can('domain.resources.manage')
  const [current, setCurrent] = useState(item),
    [name, setName] = useState(item.name),
    [target, setTarget] = useState(''),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null)
  async function mutate(kind: 'rename' | 'merge' | 'delete') {
    if (!writable || busy || (kind === 'rename' && !name.trim()) || (kind === 'merge' && !target))
      return
    const title =
      kind === 'rename'
        ? `将「${current.name}」重命名为「${name.trim()}」？`
        : kind === 'merge'
          ? `合并「${current.name}」？`
          : `删除「${current.name}」？`
    if (
      !(await confirm({
        title,
        description:
          '操作会更新当前域的题库分类与工作副本标签，不重写历史发布快照。重命名到已有名称会合并，删除不删除题目本身。',
        confirmLabel: '确认变更',
        destructive: true,
      })) ||
      !active.current
    )
      return
    setBusy(true)
    setError(null)
    try {
      if (kind === 'delete') {
        await api.deleteApiAdminTagsId(current.id)
        if (active.current) {
          toast.success('标签已删除')
          navigate('/settings/tags')
        }
        return
      }
      const result =
        kind === 'rename'
          ? await api.putApiAdminTagsId(current.id, { name: name.trim() })
          : await api.postApiAdminTagsIdMerge(current.id, { targetId: Number(target) })
      if (!active.current) return
      toast.success('标签目录与工作副本已更新')
      setCurrent(result)
      setName(result.name)
      setTarget('')
      if (result.id !== current.id) navigate(`/settings/tags/${result.id}`)
    } catch (cause) {
      if (active.current) setError(apiError(cause, '标签修改失败'))
    } finally {
      if (active.current) setBusy(false)
    }
  }
  return (
    <Card className="space-y-5 p-5">
      <h1 className="text-xl font-semibold">{current.name}</h1>
      <p className="text-sm text-muted-foreground">
        当前分类关联 {current.problemCount} 道题目；历史发布标签保持原样。
        {!writable ? '当前仅可审阅。' : ''}
      </p>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <fieldset disabled={!writable || busy} className="space-y-5">
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault()
            void mutate('rename')
          }}
        >
          <div className="min-w-0 flex-1 space-y-2">
            <Label htmlFor="tag-name">标签名称</Label>
            <Input id="tag-name" required value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          {writable && <Button type="submit">重命名</Button>}
        </form>
        <form
          className="flex flex-wrap items-end gap-3 border-t pt-4"
          onSubmit={(e) => {
            e.preventDefault()
            void mutate('merge')
          }}
        >
          <div className="min-w-0 flex-1 space-y-2">
            <Label htmlFor="tag-target">合并到本域另一个标签</Label>
            <select
              id="tag-target"
              className="h-9 w-full rounded-md border bg-background px-2 text-sm"
              value={target}
              onChange={(e) => setTarget(e.target.value)}
            >
              <option value="">选择目标标签</option>
              {catalogue
                .filter((tag) => tag.id !== current.id)
                .map((tag) => (
                  <option key={tag.id} value={tag.id}>
                    {tag.name}
                  </option>
                ))}
            </select>
          </div>
          {writable && (
            <Button type="submit" variant="outline" disabled={!target}>
              合并标签
            </Button>
          )}
        </form>
        {writable && (
          <div className="border-t pt-4">
            <Button variant="destructive" onClick={() => void mutate('delete')}>
              删除标签
            </Button>
          </div>
        )}
      </fieldset>
    </Card>
  )
}
