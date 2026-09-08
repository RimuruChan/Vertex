import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Link, useNavigate } from '@/domain/navigation'
import { useDomain } from '@/domain/DomainContext'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useActiveRef } from '@/domain/useActiveRef'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Pagination } from '@/components/ui/pagination'
import { EmptyState, PageSpinner } from '@/components/ui/misc'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { apiError, formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'
import { announcementDate, isAnnouncementPinned } from '@/lib/announcements'
import { useAnnouncementClock } from '@/hooks/useAnnouncementClock'

export default function AnnouncementListPage({ manage = false }: { manage?: boolean }) {
  const api = useDomainAPI(),
    { can } = useDomain(),
    navigate = useNavigate(),
    active = useActiveRef()
  const [params, setParams] = useSearchParams(),
    keyword = params.get('keyword') ?? '',
    value = Number(params.get('page') ?? 1),
    page = Number.isSafeInteger(value) && value > 0 ? value : 1
  const [query, setQuery] = useState(keyword),
    [title, setTitle] = useState(''),
    [open, setOpen] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null)
  useEffect(() => setQuery(keyword), [keyword])
  const load = useCallback(
    (signal: AbortSignal) =>
      manage
        ? api.getApiAdminAnnouncements({ page, size: 20, keyword }, { signal })
        : api.getApiAnnouncements({ page, size: 20, keyword }, { signal }),
    [api, page, keyword, manage],
  )
  const remote = useRemote(load),
    prefix = manage ? '/settings/announcements' : '/announcements'
  const now = useAnnouncementClock(remote.data?.items ?? [], remote.reload)
  async function create() {
    if (busy || !can('domain.resources.manage') || !title.trim()) return
    setBusy(true)
    setError(null)
    try {
      const result = await api.postApiAdminAnnouncements({ title: title.trim(), published: false })
      if (active.current) navigate(`${prefix}/${result.publicId}`)
    } catch (cause) {
      if (active.current) setError(apiError(cause, '创建公告失败'))
    } finally {
      if (active.current) setBusy(false)
    }
  }
  return (
    <div className={cn('flex flex-col gap-5', !manage && 'page-shell')}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">{manage ? '公告管理' : '域公告'}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {manage
              ? '查找、创建并进入公告详情；草稿不会出现在公开页面。'
              : '当前域发布的通知与消息。'}
          </p>
        </div>
        {manage && can('domain.resources.manage') && (
          <Button
            onClick={() => {
              setTitle('')
              setError(null)
              setOpen(true)
            }}
          >
            新建公告
          </Button>
        )}
      </div>
      <form
        className="flex max-w-xl gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          setParams(query.trim() ? { keyword: query.trim() } : {})
        }}
      >
        <Input
          aria-label="搜索公告"
          placeholder="公告标题或编号"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <Button variant="outline" type="submit">
          搜索
        </Button>
      </form>
      {remote.loading ? (
        <PageSpinner />
      ) : remote.error ? (
        <EmptyState
          title="公告加载失败"
          description={remote.error}
          action={<Button onClick={remote.reload}>重试</Button>}
        />
      ) : (
        <Card className="overflow-hidden">
          {remote.data?.items.length ? (
            <ul className="divide-y">
              {remote.data.items.map((notice) => (
                <li key={notice.id}>
                  <Link
                    to={`${prefix}/${notice.publicId}`}
                    className="flex items-center justify-between gap-3 p-4 hover:bg-muted/40"
                  >
                    <div className="min-w-0">
                      <p className="font-medium">
                        <span className="mr-2 font-mono text-xs text-muted-foreground">
                          {notice.publicId}
                        </span>
                        {notice.title}
                      </p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {isAnnouncementPinned(notice, now)
                          ? manage && notice.pinnedUntil
                            ? `置顶至 ${formatDateTime(notice.pinnedUntil)} · `
                            : '置顶 · '
                          : manage &&
                              notice.pinned &&
                              notice.pinnedUntil &&
                              Date.parse(notice.pinnedUntil) <= now
                            ? '置顶已到期 · '
                            : ''}
                        {manage ? (notice.published ? '已发布 · ' : '草稿 · ') : ''}
                        {formatDateTime(announcementDate(notice))}
                      </p>
                    </div>
                    <span className="shrink-0 text-xs text-muted-foreground">进入详情</span>
                  </Link>
                </li>
              ))}
            </ul>
          ) : (
            <EmptyState title={keyword ? '没有匹配的公告' : '暂无公告'} />
          )}
          <Pagination
            page={page}
            size={20}
            total={remote.data?.total ?? 0}
            onChange={(next) => setParams({ ...(keyword ? { keyword } : {}), page: String(next) })}
          />
        </Card>
      )}
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) setOpen(value)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>新建公告草稿</DialogTitle>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              void create()
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="new-notice-title">公告标题</Label>
              <Input
                id="new-notice-title"
                required
                maxLength={200}
                value={title}
                onChange={(e) => setTitle(e.target.value)}
              />
            </div>
            {error && (
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
            )}
            <Button type="submit" loading={busy}>
              创建并进入详情
            </Button>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
