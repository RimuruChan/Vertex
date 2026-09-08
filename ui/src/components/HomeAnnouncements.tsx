import { useCallback } from 'react'
import { ArrowRight, Pin } from 'lucide-react'
import { Link } from '@/domain/navigation'
import { useDomainAPI } from '@/domain/useDomainAPI'
import { useRemote } from '@/domain/useRemote'
import { useAnnouncementClock } from '@/hooks/useAnnouncementClock'
import { announcementDate, homeAnnouncements, isAnnouncementPinned } from '@/lib/announcements'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/misc'

export default function HomeAnnouncements({ className }: { className?: string }) {
  const api = useDomainAPI()
  const remote = useRemote(
    useCallback(
      async (signal: AbortSignal) => {
        const [pinned, latest] = await Promise.all([
          api.getApiAnnouncements({ pinned: true, size: 2 }, { signal }),
          api.getApiAnnouncements({ pinned: false, size: 5 }, { signal }),
        ])
        return [...pinned.items, ...latest.items]
      },
      [api],
    ),
  )
  const now = useAnnouncementClock(remote.data ?? [], remote.reload)
  const items = homeAnnouncements(remote.data ?? [], now)
  return (
    <section
      className={cn('surface-panel overflow-hidden', className)}
      aria-labelledby="home-announcements-title"
    >
      <div className="flex items-center justify-between border-b border-border px-5 py-4">
        <h2 id="home-announcements-title" className="text-sm font-semibold">
          公告
        </h2>
        <Link
          to="/announcements"
          aria-label="查看全部公告"
          className="inline-flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-primary"
        >
          全部 <ArrowRight className="size-3.5" />
        </Link>
      </div>
      {remote.error ? (
        <div className="space-y-3 p-5 text-sm" role="alert">
          <p className="text-muted-foreground">{remote.error}</p>
          <Button size="sm" variant="outline" onClick={remote.reload}>
            重新加载
          </Button>
        </div>
      ) : remote.loading && !remote.data ? (
        <div className="space-y-5 p-5" aria-label="正在加载公告">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-10" />
          ))}
        </div>
      ) : items.length ? (
        <ul className="divide-y divide-border/60">
          {items.map((notice, index) => {
            const pinned = isAnnouncementPinned(notice, now)
            const date = announcementDate(notice)
            return (
              <li key={notice.id} className={index > 2 ? 'hidden lg:block' : undefined}>
                <Link
                  to={`/announcements/${notice.publicId}`}
                  className="group block px-5 py-3.5 transition-colors hover:bg-muted/40"
                >
                  <p className="line-clamp-2 text-sm font-medium leading-6 group-hover:text-primary">
                    {notice.title}
                  </p>
                  <div className="mt-1.5 flex items-center gap-2 text-xs text-muted-foreground">
                    {pinned && (
                      <span className="inline-flex items-center gap-1 text-primary">
                        <Pin className="size-3" />
                        置顶
                      </span>
                    )}
                    <time dateTime={date} title={formatDateTime(date)}>
                      {new Date(date).toLocaleDateString(undefined, {
                        month: 'short',
                        day: 'numeric',
                      })}
                    </time>
                  </div>
                </Link>
              </li>
            )
          })}
        </ul>
      ) : (
        <p className="px-5 py-6 text-sm text-muted-foreground">暂无公告，新的消息会出现在这里。</p>
      )}
    </section>
  )
}
