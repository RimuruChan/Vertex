import { useCallback, useState } from 'react'
import { useParams } from 'react-router-dom'
import type { DtoAnnouncementResponse } from '@/generated/api/model'
import { Link, useNavigate } from '@/domain/navigation'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useDomain } from '@/domain/DomainContext'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { useCanonicalResourcePath } from '@/hooks/useCanonicalPath'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Textarea } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState, PageSpinner } from '@/components/ui/misc'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { useToast } from '@/components/ui/toast'
import MdRenderer from '@/components/MdRenderer'
import { apiError, formatDateTime } from '@/lib/format'

export default function AnnouncementDetailPage({ manage = false }: { manage?: boolean }) {
  const { id = '' } = useParams(),
    api = useDomainAPI()
  const load = useCallback(
    (signal: AbortSignal) =>
      manage
        ? api.getApiAdminAnnouncementsId(id, { signal })
        : api.getApiAnnouncementsId(id, { signal }),
    [api, id, manage],
  )
  const remote = useRemote(load),
    prefix = manage ? 'settings/announcements' : 'announcements'
  useCanonicalResourcePath(prefix, id, remote.data)
  if (remote.error)
    return (
      <EmptyState
        title="公告不可用"
        description={remote.error}
        action={<Button onClick={remote.reload}>重试</Button>}
      />
    )
  if (!remote.data) return <PageSpinner />
  const notice = remote.data
  return (
    <div className={manage ? 'space-y-4' : 'page-shell'}>
      <Link className="text-sm text-muted-foreground hover:text-primary" to={`/${prefix}`}>
        返回公告列表
      </Link>
      {manage ? (
        <AnnouncementEditor key={notice.id} notice={notice} />
      ) : (
        <Card className="space-y-4 p-5 sm:p-7">
          <h1 className="text-2xl font-semibold">{notice.title}</h1>
          <p className="text-xs text-muted-foreground">
            {notice.authorName || '域公告'} · {formatDateTime(notice.updatedAt)}
          </p>
          <MdRenderer content={notice.contentMd} />
        </Card>
      )}
    </div>
  )
}

function AnnouncementEditor({ notice }: { notice: DtoAnnouncementResponse }) {
  const [draft, setDraft] = useState(notice),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null)
  const api = useDomainAPI(),
    { can, domain } = useDomain(),
    active = useActiveRef(),
    confirm = useConfirm(),
    toast = useToast(),
    navigate = useNavigate(),
    writable = can('domain.resources.manage')
  async function save() {
    if (busy || !writable) return
    setBusy(true)
    setError(null)
    try {
      const result = await api.putApiAdminAnnouncementsId(notice.id, {
        title: draft.title,
        contentMd: draft.contentMd,
        pinned: draft.pinned,
        published: draft.published,
      })
      if (!active.current) return
      setDraft(result)
      toast.success(result.published ? '公告已发布' : '草稿已保存')
    } catch (cause) {
      if (active.current) setError(apiError(cause, '保存公告失败'))
    } finally {
      if (active.current) setBusy(false)
    }
  }
  async function remove() {
    if (
      busy ||
      !writable ||
      !(await confirm({
        title: `删除「${draft.title}」？`,
        description: '公告会永久删除；只想暂时隐藏时可取消发布并保存。',
        confirmLabel: '删除公告',
        destructive: true,
      }))
    )
      return
    if (!active.current) return
    setBusy(true)
    setError(null)
    try {
      await api.deleteApiAdminAnnouncementsId(notice.id)
      if (active.current) {
        toast.success('公告已删除')
        navigate('/settings/announcements')
      }
    } catch (cause) {
      if (active.current) setError(apiError(cause, '删除公告失败'))
    } finally {
      if (active.current) setBusy(false)
    }
  }
  return (
    <div className="space-y-4">
      <Card className="space-y-4 p-5">
        <h1 className="text-xl font-semibold">公告详情 · {notice.publicId}</h1>
        {domain.archived && (
          <p className="text-sm text-muted-foreground">域已归档，当前仅可审阅。</p>
        )}
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            void save()
          }}
        >
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <fieldset disabled={!writable || busy} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="notice-title">标题</Label>
              <Input
                id="notice-title"
                required
                maxLength={200}
                value={draft.title}
                onChange={(e) => setDraft({ ...draft, title: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="notice-content">正文（Markdown）</Label>
              <Textarea
                id="notice-content"
                rows={12}
                value={draft.contentMd}
                onChange={(e) => setDraft({ ...draft, contentMd: e.target.value })}
              />
            </div>
            <div className="flex flex-wrap gap-5 text-sm">
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={draft.pinned}
                  onChange={(e) => setDraft({ ...draft, pinned: e.target.checked })}
                />
                置顶
              </label>
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={draft.published}
                  onChange={(e) => setDraft({ ...draft, published: e.target.checked })}
                />
                发布到当前域
              </label>
            </div>
          </fieldset>
          {writable && (
            <Button loading={busy} type="submit">
              保存公告
            </Button>
          )}
        </form>
        <p className="text-xs text-muted-foreground">
          修改发布状态需保存才生效。作者：{draft.authorName || '域公告'}；最近保存{' '}
          {formatDateTime(draft.updatedAt)}。
        </p>
      </Card>
      <Card className="space-y-3 p-5">
        <h2 className="text-sm font-medium">正文预览</h2>
        <MdRenderer content={draft.contentMd || '尚未填写正文。'} />
      </Card>
      {writable && (
        <div className="flex justify-end">
          <Button variant="destructive" onClick={() => void remove()} disabled={busy}>
            删除公告
          </Button>
        </div>
      )}
    </div>
  )
}
