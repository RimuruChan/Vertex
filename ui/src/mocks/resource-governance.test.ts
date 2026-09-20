import { authoringFixture } from './authoring-fixture'
import { describe, expect, it } from 'vitest'
import type { DtoAnnouncementResponse, DtoTagCatalogResponse, DtoProblemResponse } from './models'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, juryUser, demoUser } from './identities'

describe('domain notices and taxonomy', () => {
  it('applies pin deadlines and preserves the first publication time across edits and republishing', () => {
    let now = Date.parse('2026-09-08T10:00:00Z')
    const api = createMockAPI(createFixtures(), () => now)
    api.state.user = { ...adminUser }
    const path = '/api/domains/training/admin/announcements'
    const pinned = api.handle({
      method: 'POST',
      path,
      body: {
        title: 'PinTest old',
        pinned: true,
        pinnedUntil: new Date(now + 3 * 3600000).toISOString(),
      },
    }) as DtoAnnouncementResponse
    const first = pinned.publishedAt
    now += 3600000
    const recent = api.handle({
      method: 'POST',
      path,
      body: { title: 'PinTest recent' },
    }) as DtoAnnouncementResponse
    const draft = api.handle({
      method: 'POST',
      path,
      body: { title: 'PinTest draft', published: false, pinned: true },
    }) as DtoAnnouncementResponse
    expect(draft.publishedAt).toBeUndefined()
    const feed = (pinned?: boolean) =>
      api.handle({
        method: 'GET',
        path: '/api/domains/training/announcements',
        params: { keyword: 'PinTest', pinned },
      }) as { items: DtoAnnouncementResponse[]; total: number }
    expect(feed().items.map((item) => item.id)).toEqual([pinned.id, recent.id])
    now += 2 * 3600000
    expect(feed(true).total).toBe(0)
    expect(feed(false).items.map((item) => item.id)).toEqual([recent.id, pinned.id])
    const hidden = api.handle({
      method: 'PUT',
      path: `${path}/${pinned.id}`,
      body: { ...pinned, published: false },
    }) as DtoAnnouncementResponse
    const republished = api.handle({
      method: 'PUT',
      path: `${path}/${pinned.id}`,
      body: { ...hidden, title: 'PinTest edited', published: true, pinned: false },
    }) as DtoAnnouncementResponse
    expect(republished.publishedAt).toBe(first)
    expect(republished.pinnedUntil).toBeUndefined()
    expect(feed().items.map((item) => item.id)).toEqual([recent.id, pinned.id])
  })

  it('keeps empty and error simulations explicit without bypassing governance', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...juryUser }
    api.scenario = 'empty'
    expect(api.handle({ method: 'GET', path: '/api/domains/training/admin/tags' })).toEqual({
      items: [],
      total: 0,
    })
    api.state.user = { ...demoUser }
    expect(() => api.handle({ method: 'GET', path: '/api/domains/training/admin/tags' })).toThrow(
      '资源管理',
    )
    api.scenario = 'error'
    expect(() =>
      api.handle({ method: 'GET', path: '/api/domains/training/announcements' }),
    ).toThrow('加载失败')
  })
  it('allows a domain owner without site-admin status and never leaks drafts through the public feed', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...juryUser }
    const notice = api.handle({
      method: 'POST',
      path: '/api/domains/training/admin/announcements',
      body: { title: 'Draft', published: false },
    }) as DtoAnnouncementResponse
    expect(notice.id).toBe('2')
    expect(() =>
      api.handle({ method: 'GET', path: `/api/domains/training/announcements/${notice.id}` }),
    ).toThrow()
    expect(
      api.handle({
        method: 'GET',
        path: `/api/domains/training/admin/announcements/${notice.id}`,
      }),
    ).toHaveProperty('title', 'Draft')
    api.state.user = { ...demoUser }
    expect(() =>
      api.handle({ method: 'GET', path: '/api/domains/training/admin/announcements' }),
    ).toThrow('资源管理')
    expect(() =>
      api.handle({
        method: 'POST',
        path: '/api/domains/training/admin/tags',
        body: { name: 'No' },
      }),
    ).toThrow()
    expect(
      api.handle({
        method: 'GET',
        path: '/api/domains/training/announcements',
        params: { keyword: 'Draft' },
      }),
    ).toMatchObject({ items: [], total: 0 })
  })
  it('keeps numbered announcements and totals local when searching, publishing, and deleting', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...adminUser }
    const create = (domain: string, published: boolean) =>
      api.handle({
        method: 'POST',
        path: `/api/domains/${domain}/admin/announcements`,
        body: { title: `Scoped notice ${domain}`, published },
      }) as DtoAnnouncementResponse
    const a = create('official', true),
      b = create('training', true)
    create('training', false)
    expect(a.id).toBe(b.id)
    expect(a.title).not.toBe(b.title)
    expect(
      api.handle({
        method: 'GET',
        path: '/api/domains/training/admin/announcements',
        params: { keyword: 'Scoped', size: 1 },
      }),
    ).toMatchObject({ total: 2, items: expect.any(Array) })
    expect(
      api.handle({
        method: 'GET',
        path: '/api/domains/training/announcements',
        params: { keyword: 'Scoped' },
      }),
    ).toMatchObject({ total: 1, items: [{ id: b.id }] })
    expect(
      api.handle({ method: 'GET', path: `/api/domains/official/admin/announcements/${b.id}` }),
    ).toMatchObject({ title: a.title })
    api.handle({ method: 'DELETE', path: `/api/domains/training/admin/announcements/${b.id}` })
    expect(
      api.handle({ method: 'GET', path: `/api/domains/official/announcements/${a.id}` }),
    ).toHaveProperty('id', a.id)
  })
  it('merges current classifications without rewriting private copies or immutable releases', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...adminUser }
    const problem = api.handle({
      method: 'POST',
      path: '/api/domains/official/admin/problems',
      body: { title: 'Taxonomy fixture', tags: ['qa-old', 'qa-new'] },
    }) as DtoProblemResponse
    const path = `/api/domains/official/admin/problems/${problem.id}`
    const fixture = authoringFixture(api, problem.id)
    const check = fixture.check()
    fixture.publish(fixture.commit().revision, check.id)
    const before = fixture.copy()
    const tags = (
      api.handle({ method: 'GET', path: '/api/domains/official/admin/tags' }) as {
        items: DtoTagCatalogResponse[]
      }
    ).items
    const old = tags.find((t) => t.name === 'qa-old')!,
      target = tags.find((t) => t.name === 'qa-new')!
    api.handle({
      method: 'POST',
      path: `/api/domains/official/admin/tags/${old.id}/merge`,
      body: { targetId: target.id },
    })
    expect(api.handle({ method: 'GET', path })).toHaveProperty('tags', ['qa-new'])
    expect(fixture.copy()).toEqual(before)
    expect(api.state.problemReleases[problem.id][0].problem.tags).toEqual(['qa-old', 'qa-new'])
    api.handle({ method: 'DELETE', path: `/api/domains/official/admin/tags/${target.id}` })
    expect(api.handle({ method: 'GET', path })).toHaveProperty('tags', [])
  })
  it('withdraws resource governance on suspension and keeps archived management read-only', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...juryUser }
    api.handle({
      method: 'PUT',
      path: '/api/domains/training/members/demo',
      body: { roleKey: 'admin', status: 'active' },
    })
    api.state.user = { ...demoUser }
    const tag = api.handle({
      method: 'POST',
      path: '/api/domains/training/admin/tags',
      body: { name: 'governed' },
    }) as DtoTagCatalogResponse
    api.state.user = { ...juryUser }
    api.handle({
      method: 'PUT',
      path: '/api/domains/training/members/demo',
      body: { roleKey: 'admin', status: 'suspended' },
    })
    api.state.user = { ...demoUser }
    expect(() =>
      api.handle({ method: 'DELETE', path: `/api/domains/training/admin/tags/${tag.id}` }),
    ).toThrow()
    api.state.user = { ...juryUser }
    api.handle({ method: 'PUT', path: '/api/domains/training/archive', body: { archived: true } })
    expect(
      api.handle({ method: 'GET', path: `/api/domains/training/admin/tags/${tag.id}` }),
    ).toHaveProperty('name', 'governed')
    expect(() =>
      api.handle({ method: 'DELETE', path: `/api/domains/training/admin/tags/${tag.id}` }),
    ).toThrow('归档')
  })
})
