import { describe, expect, it } from 'vitest'
import type {
  DtoAnnouncementResponse,
  DtoTagCatalogResponse,
  DtoProblemResponse,
  DtoWorkspaceResponse,
} from '@/generated/api/model'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, juryUser, demoUser } from './identities'

describe('domain notices and taxonomy', () => {
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
    expect(notice.publicId).toBe('2')
    expect(() =>
      api.handle({ method: 'GET', path: `/api/domains/training/announcements/${notice.publicId}` }),
    ).toThrow()
    expect(
      api.handle({
        method: 'GET',
        path: `/api/domains/training/admin/announcements/${notice.publicId}`,
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
        body: { title: 'Scoped notice', published },
      }) as DtoAnnouncementResponse
    const a = create('official', true),
      b = create('training', true)
    create('training', false)
    expect(a.publicId).toBe(b.publicId)
    expect(a.id).not.toBe(b.id)
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
    expect(() =>
      api.handle({ method: 'GET', path: `/api/domains/official/admin/announcements/${b.id}` }),
    ).toThrow()
    api.handle({ method: 'DELETE', path: `/api/domains/training/admin/announcements/${b.id}` })
    expect(api.handle({ method: 'GET', path: `/api/announcements/${a.publicId}` })).toHaveProperty(
      'id',
      a.id,
    )
  })
  it('merges current classifications and working labels without changing an old release', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...adminUser }
    const problem = api.handle({
      method: 'POST',
      path: '/api/admin/problems',
      body: { title: 'Taxonomy fixture', tags: ['qa-old', 'qa-new'] },
    }) as DtoProblemResponse
    const path = `/api/admin/problems/${problem.id}`
    api.handle({
      method: 'PUT',
      path: path + '/statements/zh',
      body: { name: 'Taxonomy fixture', legend: 'Fixture statement' },
    })
    api.handle({
      method: 'POST',
      path: path + '/testdata',
      body: { file: new Blob(['fixture']), checker: 'diff' },
    })
    const { meta } = api.handle({ method: 'GET', path: path + '/package' }) as DtoWorkspaceResponse
    api.handle({
      method: 'POST',
      path: path + '/publish',
      body: { revision: meta.packageRevision, artifactVersion: meta.testdataVersion },
    })
    const tags = (
      api.handle({ method: 'GET', path: '/api/admin/tags' }) as { items: DtoTagCatalogResponse[] }
    ).items
    const old = tags.find((t) => t.name === 'qa-old')!,
      target = tags.find((t) => t.name === 'qa-new')!
    api.handle({
      method: 'POST',
      path: `/api/admin/tags/${old.id}/merge`,
      body: { targetId: target.id },
    })
    expect(api.handle({ method: 'GET', path })).toHaveProperty('tags', ['qa-new'])
    expect(
      (api.handle({ method: 'GET', path: path + '/package' }) as DtoWorkspaceResponse).meta
        .packageRevision,
    ).toBe(meta.packageRevision + 1)
    expect(api.state.problemReleases[problem.id][0].problem.tags).toEqual(['qa-old', 'qa-new'])
    api.handle({ method: 'DELETE', path: `/api/admin/tags/${target.id}` })
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
