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
import { DateTimePicker } from '@/components/ui/date-time-picker'
import { Label } from '@/components/ui/label'
import { EmptyState, PageSpinner } from '@/components/ui/misc'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { useToast } from '@/components/ui/toast'
import MdRenderer from '@/components/MdRenderer'
import { apiError, formatDateTime, fromLocalInput, toLocalInput } from '@/lib/format'
import { cn } from '@/lib/utils'
import { announcementDate, isAnnouncementPinned } from '@/lib/announcements'
import { useAnnouncementClock } from '@/hooks/useAnnouncementClock'
import { Pin } from 'lucide-react'

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
  const now = useAnnouncementClock(remote.data ? [remote.data] : [])
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
    <div className={cn('flex flex-col gap-5', !manage && 'page-shell')}>
      <Link className="w-fit text-sm text-muted-foreground hover:text-primary" to={`/${prefix}`}>
        返回公告列表
      </Link>
      {manage ? (
        <AnnouncementEditor key={notice.id} notice={notice} />
      ) : (
        <Card className="space-y-4 p-5 sm:p-7">
          <h1 className="text-2xl font-semibold">{notice.title}</h1>
          <p className="text-xs text-muted-foreground">
            {isAnnouncementPinned(notice, now) && (
              <span className="mr-2 inline-flex items-center gap-1 text-primary">
                <Pin className="size-3" />
                置顶
              </span>
            )}
            {notice.authorName || '域公告'} · {formatDateTime(announcementDate(notice))}
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
  const [pinMode, setPinMode] = useState<'none' | 'timed' | 'forever'>(
    notice.pinned ? (notice.pinnedUntil ? 'timed' : 'forever') : 'none',
  )
  const [pinDeadline, setPinDeadline] = useState(
    toLocalInput(notice.pinnedUntil ?? new Date(Date.now() + 7 * 86400000).toISOString()),
  )
  const now = useAnnouncementClock([draft])
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
        pinned: pinMode !== 'none',
        pinnedUntil: pinMode === 'timed' ? fromLocalInput(pinDeadline) : undefined,
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
            <fieldset className="space-y-4 rounded-lg border border-border p-4">
              <legend className="px-1 text-sm font-medium">公告置顶</legend>
              <div className="grid gap-2 sm:grid-cols-3">
                {(
                  [
                    ['none', '不置顶'],
                    ['timed', '限时置顶'],
                    ['forever', '长期置顶'],
                  ] as const
                ).map(([mode, label]) => (
                  <label
                    key={mode}
                    className={cn(
                      'flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2.5 text-sm',
                      pinMode === mode
                        ? 'border-primary/40 bg-primary/5 text-primary'
                        : 'border-border text-muted-foreground',
                    )}
                  >
                    <input
                      type="radio"
                      name="notice-pin-mode"
                      value={mode}
                      checked={pinMode === mode}
                      onChange={() => setPinMode(mode)}
                      className="accent-primary"
                    />
                    {label}
                  </label>
                ))}
              </div>
              {pinMode === 'timed' ? (
                <div className="space-y-3">
                  <div className="space-y-2">
                    <Label htmlFor="notice-pin-until">置顶截止时间</Label>
                    <DateTimePicker
                      id="notice-pin-until"
                      disabled={!writable || busy}
                      required
                      value={pinDeadline}
                      onChange={setPinDeadline}
                      className="max-w-xs"
                    />
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    {[1, 7].map((days) => (
                      <Button
                        key={days}
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() =>
                          setPinDeadline(
                            toLocalInput(new Date(Date.now() + days * 86400000).toISOString()),
                          )
                        }
                      >
                        {days} 天
                      </Button>
                    ))}
                    <span className="text-xs text-muted-foreground">
                      时区：{Intl.DateTimeFormat().resolvedOptions().timeZone}
                    </span>
                  </div>
                  <p className="text-xs leading-5 text-muted-foreground">
                    {Date.parse(fromLocalInput(pinDeadline) ?? '') <= now
                      ? '置顶时间已到期，公告按普通顺序展示。可选择新的截止时间延长置顶。'
                      : '到期后自动恢复普通排序，公告继续保留。'}
                  </p>
                </div>
              ) : (
                <p className="text-xs text-muted-foreground">
                  {pinMode === 'forever'
                    ? '发布后保持置顶，直到手动取消。'
                    : '按首次发布时间排序，编辑正文不会改变位置。'}
                </p>
              )}
            </fieldset>
            <div className="flex flex-wrap gap-5 text-sm">
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
          发布和置顶设置需保存才生效。作者：{draft.authorName || '域公告'}；最近保存{' '}
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
