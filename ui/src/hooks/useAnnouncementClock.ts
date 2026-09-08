import { useEffect, useRef, useState } from 'react'
import type { DtoAnnouncementResponse } from '@/generated/api/model'

// Refresh open feeds at their next pin deadline without polling or a server job.
export function useAnnouncementClock(items: DtoAnnouncementResponse[], onExpire?: () => void) {
  const [, tick] = useState(0)
  const reload = useRef(onExpire)
  reload.current = onExpire
  const now = Date.now()
  const deadline = Math.min(
    ...items
      .filter((item) => item.pinned && item.published && item.pinnedUntil)
      .map((item) => Date.parse(item.pinnedUntil!))
      .filter((time) => time > now),
  )
  useEffect(() => {
    if (!Number.isFinite(deadline)) return
    const timer = window.setTimeout(
      () => {
        tick((value) => value + 1)
        reload.current?.()
      },
      Math.min(Math.max(0, deadline - Date.now() + 25), 2_147_483_647),
    )
    return () => window.clearTimeout(timer)
  }, [deadline])
  return now
}
