import { describe, expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, demoUser } from './identities'
import type {
  DomainLibraryPage,
  DomainWorkingCopy,
  DomainMaterialView,
} from '@/generated/api/model'

describe('authoring library privacy', () => {
  it('searches my current draft while exposing only shared titles to collaborators', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...adminUser }
    const root = '/api/domains/official/authoring/problems',
      path = root + '/1000'
    api.handle({
      method: 'PUT',
      path: '/api/domains/official/admin/problems/1000/access',
      body: { username: demoUser.username, role: 'reader' },
    })
    const copy = api.handle({ method: 'POST', path: path + '/working-copy' }) as DomainWorkingCopy
    const meta = api.handle({
      method: 'GET',
      path: path + '/materials/problem',
    }) as DomainMaterialView
    api.handle({
      method: 'PUT',
      path: path + '/working-copy/entries/problem/text',
      body: {
        etag: copy.etag,
        text: JSON.stringify({ ...meta.metadata, title: 'private-title-af33' }),
      },
    })
    const own = api.handle({
      method: 'GET',
      path: root,
      params: { keyword: 'af33', status: 'changes' },
    }) as DomainLibraryPage
    expect(own.total).toBe(1)
    expect(own.items[0]).toMatchObject({
      id: '1000',
      title: 'private-title-af33',
      hasChanges: true,
      hasCopy: true,
      headRevision: 0,
    })
    expect(JSON.stringify(own)).not.toContain('etag')
    expect(JSON.stringify(own)).not.toContain('entries')
    api.state.user = { ...demoUser }
    expect(api.handle({ method: 'GET', path: root, params: { keyword: 'af33' } })).toEqual({
      items: [],
      total: 0,
    })
    const shared = api.handle({
      method: 'GET',
      path: root,
      params: { keyword: '1000' },
    }) as DomainLibraryPage
    expect(shared.items[0]).toMatchObject({ title: '两数之和', hasCopy: false, canEdit: false })
    expect(() => api.handle({ method: 'GET', path: path + '/working-copy' })).toThrow('尚未创建')
  })
})
