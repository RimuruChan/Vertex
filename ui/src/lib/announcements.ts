import type { DtoAnnouncementResponse as Announcement } from '@/generated/api/model'

export function isAnnouncementPinned(notice: Announcement, now = Date.now()) {
  return (
    notice.published &&
    notice.pinned &&
    (!notice.pinnedUntil || Date.parse(notice.pinnedUntil) > now)
  )
}

export function announcementDate(notice: Announcement) {
  return notice.publishedAt ?? notice.createdAt
}

export function compareAnnouncements(a: Announcement, b: Announcement, now = Date.now()) {
  return (
    Number(isAnnouncementPinned(b, now)) - Number(isAnnouncementPinned(a, now)) ||
    Date.parse(announcementDate(b)) - Date.parse(announcementDate(a)) ||
    b.id.localeCompare(a.id)
  )
}

export function homeAnnouncements(items: Announcement[], now = Date.now()) {
  const unique = [
    ...new Map(items.filter((item) => item.published).map((item) => [item.id, item])).values(),
  ].sort((a, b) => compareAnnouncements(a, b, now))
  const pinned = unique.filter((item) => isAnnouncementPinned(item, now)).slice(0, 2)
  const latest = unique.filter((item) => !isAnnouncementPinned(item, now))
  return [...pinned, ...latest.slice(0, 5 - pinned.length)]
}
