import { describe, expect, it } from 'vitest'
import type { DtoAnnouncementResponse as Announcement } from '@/generated/api/model'
import { compareAnnouncements, homeAnnouncements, isAnnouncementPinned } from './announcements'

const now = Date.parse('2026-09-08T10:00:00Z')
const notice = (id: string, props: Partial<Announcement> = {}): Announcement => ({
  id,
  publicId: id,
  title: id,
  contentMd: '',
  pinned: false,
  published: true,
  createdAt: '2026-09-01T00:00:00Z',
  updatedAt: '2026-09-01T00:00:00Z',
  ...props,
})

describe('announcement presentation', () => {
  it('expires a pin exactly at its deadline and never pins drafts', () => {
    expect(isAnnouncementPinned(notice('1', { pinned: true }), now)).toBe(true)
    expect(
      isAnnouncementPinned(
        notice('1', { pinned: true, pinnedUntil: new Date(now + 1).toISOString() }),
        now,
      ),
    ).toBe(true)
    expect(
      isAnnouncementPinned(
        notice('1', { pinned: true, pinnedUntil: new Date(now).toISOString() }),
        now,
      ),
    ).toBe(false)
    expect(isAnnouncementPinned(notice('1', { pinned: true, published: false }), now)).toBe(false)
  })

  it('reserves room for recent notices without duplicating rows or exposing drafts', () => {
    const pins = [1, 2, 3, 4].map((i) => notice(`pin-${i}`, { pinned: true }))
    const ordinary = [1, 2, 3, 4, 5].map((i) =>
      notice(`recent-${i}`, { publishedAt: new Date(now - i * 1000).toISOString() }),
    )
    const items = homeAnnouncements(
      [...pins, ...ordinary, ordinary[0], notice('draft', { published: false })],
      now,
    )
    expect(items).toHaveLength(5)
    expect(items.filter((item) => isAnnouncementPinned(item, now))).toHaveLength(2)
    expect(items.slice(2).map((item) => item.id)).toEqual(['recent-1', 'recent-2', 'recent-3'])
  })

  it('keeps edits in the publication timeline and retains expired notices', () => {
    const older = notice('old', {
      pinned: true,
      pinnedUntil: new Date(now - 1).toISOString(),
      updatedAt: new Date(now).toISOString(),
    })
    const recent = notice('new', { publishedAt: new Date(now - 1000).toISOString() })
    expect(
      [older, recent].sort((a, b) => compareAnnouncements(a, b, now)).map((item) => item.id),
    ).toEqual(['new', 'old'])
    expect(homeAnnouncements([older, recent], now).map((item) => item.id)).toEqual(['new', 'old'])
  })
})
