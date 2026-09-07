import { describe, expect, it } from 'vitest'
import type {
  DtoProblemResponse,
  DtoWorkspaceResponse,
  DtoProblemGrantResponse,
  DtoContestGrantResponse,
  DtoContestResponse,
  DtoSetResponse,
} from '@/generated/api/model'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, demoUser, observerUser, contestantUser, juryUser } from './identities'

describe('resource collaboration and dynamic group inheritance', () => {
  it('keeps a reader grant after direct edit access is removed, then withdraws group inheritance immediately', () => {
    const api = createMockAPI(createFixtures())
    const p = api.handle({
      method: 'POST',
      path: '/api/domains/training/admin/problems',
      body: { title: 'Private collaboration' },
    }) as DtoProblemResponse
    const path = `/api/domains/training/admin/problems/${p.publicId}`
    api.handle({ method: 'PUT', path: path + '/access', body: { group: '1', role: 'reader' } })
    api.handle({
      method: 'PUT',
      path: path + '/access',
      body: { username: 'observer', role: 'editor' },
    })
    api.state.user = { ...observerUser }
    const workspace = () =>
      api.handle({ method: 'GET', path: path + '/package' }) as DtoWorkspaceResponse
    expect(workspace().meta.canEdit).toBe(true)
    expect(workspace().meta.canPublish).toBe(false)
    expect(() =>
      api.handle({
        method: 'PUT',
        path: path + '/access',
        body: { username: 'contestant', role: 'editor' },
      }),
    ).toThrow()
    api.state.user = { ...demoUser }
    const grants = api.handle({ method: 'GET', path: path + '/access' }) as {
      items: DtoProblemGrantResponse[]
    }
    api.handle({
      method: 'DELETE',
      path: path + '/access/' + grants.items.find((g) => g.userId === observerUser.id)!.id,
    })
    api.state.user = { ...observerUser }
    expect(workspace().meta.canEdit).toBe(false)
    expect(() =>
      api.handle({
        method: 'PUT',
        path: path + '/statements/zh',
        body: { name: 'No', legend: 'No' },
      }),
    ).toThrow()
    api.state.user = { ...demoUser }
    api.handle({ method: 'DELETE', path: '/api/domains/training/groups/1/members/observer' })
    api.state.user = { ...observerUser }
    expect(() => workspace()).toThrow()
  })
  it('transfers ownership without preserving creator privileges or changing the public number', () => {
    const api = createMockAPI(createFixtures())
    const p = api.handle({
      method: 'POST',
      path: '/api/domains/training/admin/problems',
      body: { title: 'Transfer' },
    }) as DtoProblemResponse
    const path = `/api/domains/training/admin/problems/${p.publicId}`
    api.handle({ method: 'PUT', path: path + '/owner', body: { username: 'observer' } })
    expect(() => api.handle({ method: 'GET', path })).toThrow()
    api.state.user = { ...observerUser }
    expect(api.handle({ method: 'GET', path })).toMatchObject({
      ownerId: observerUser.id,
      ownerName: 'observer',
      authorId: demoUser.id,
      publicId: p.publicId,
    })
    expect(() =>
      api.handle({
        method: 'PUT',
        path: path + '/access',
        body: { username: 'lin', role: 'editor' },
      }),
    ).toThrow('本域有效')
  })
  it('combines contest roles and recomputes the effective roster after grants and membership changes', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...adminUser }
    const event = api.handle({
      method: 'POST',
      path: '/api/admin/contests',
      body: {
        title: 'Permissions',
        beginAt: '2030-01-01T00:00:00Z',
        endAt: '2030-01-02T00:00:00Z',
        visibility: 'private',
      },
    }) as DtoContestResponse
    const group = api.handle({
      method: 'POST',
      path: '/api/domains/official/groups',
      body: { name: 'Staff' },
    }) as { publicId: string }
    api.handle({
      method: 'PUT',
      path: `/api/domains/official/groups/${group.publicId}/members/observer`,
      body: { role: 'member' },
    })
    const path = `/api/contests/${event.publicId}`
    api.handle({
      method: 'PUT',
      path: path + '/access',
      body: { group: group.publicId, role: 'observer' },
    })
    api.handle({
      method: 'PUT',
      path: path + '/access',
      body: { group: group.publicId, role: 'jury' },
    })
    api.state.user = { ...observerUser }
    expect(api.handle({ method: 'GET', path })).toMatchObject({
      staffRole: 'jury',
      contest: { permissions: { viewJury: true, rejudge: true, manageAccess: false } },
    })
    api.state.user = { ...adminUser }
    const grants = api.handle({ method: 'GET', path: path + '/access' }) as {
      items: DtoContestGrantResponse[]
    }
    api.handle({
      method: 'DELETE',
      path: path + '/access/' + grants.items.find((g) => g.role === 'jury')!.id,
    })
    api.state.user = { ...observerUser }
    expect(api.handle({ method: 'GET', path })).toMatchObject({
      staffRole: 'observer',
      contest: { permissions: { viewJury: true, rejudge: false, submit: false } },
    })
    expect(api.handle({ method: 'GET', path: '/api/admin/contests' })).toMatchObject({
      items: expect.arrayContaining([expect.objectContaining({ id: event.id })]),
    })
    expect(
      api.handle({ method: 'GET', path: `/api/admin/contests/${event.publicId}` }),
    ).toMatchObject({
      contest: { permissions: { edit: false, viewJury: true } },
    })
    api.state.user = { ...adminUser }
    api.handle({
      method: 'DELETE',
      path: `/api/domains/official/groups/${group.publicId}/members/observer`,
    })
    api.state.user = { ...observerUser }
    expect(() => api.handle({ method: 'GET', path })).toThrow()
  })
  it('uses group access for sets and revokes it on suspension or group removal', () => {
    const api = createMockAPI(createFixtures())
    const set = api.handle({
      method: 'POST',
      path: '/api/domains/training/problem-sets',
      body: { title: 'Shared set', visibility: 'private' },
    }) as DtoSetResponse
    const path = `/api/domains/training/problem-sets/${set.publicId}`
    api.handle({ method: 'PUT', path: path + '/access', body: { group: '1', role: 'editor' } })
    api.state.user = { ...observerUser }
    expect(api.handle({ method: 'GET', path })).toMatchObject({
      permissions: { edit: true, manageAccess: false, publish: false },
    })
    api.state.user = { ...juryUser }
    api.handle({
      method: 'PUT',
      path: '/api/domains/training/members/observer',
      body: { roleKey: 'viewer', status: 'suspended' },
    })
    api.state.user = { ...observerUser }
    expect(() => api.handle({ method: 'GET', path })).toThrow()
    api.state.user = { ...juryUser }
    api.handle({
      method: 'PUT',
      path: '/api/domains/training/members/observer',
      body: { roleKey: 'viewer', status: 'active' },
    })
    api.handle({ method: 'DELETE', path: '/api/domains/training/groups/1' })
    api.state.user = { ...observerUser }
    expect(() => api.handle({ method: 'GET', path })).toThrow()
  })
  it('rejects another domain group and preserves complete test input when reading a single definition', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...adminUser }
    const p = api.state.problems[0],
      path = `/api/admin/problems/${p.publicId}`
    const foreign = api.handle({ method: 'GET', path: '/api/domains/training/groups/1' }) as {
      id: string
    }
    expect(() =>
      api.handle({
        method: 'PUT',
        path: path + '/access',
        body: { group: foreign.id, role: 'reader' },
      }),
    ).toThrow()
    const input = '1 2 3\n'.repeat(300)
    api.handle({
      method: 'PUT',
      path: path + '/tests/1',
      body: { source: 'manual', inputData: input },
    })
    expect(api.handle({ method: 'GET', path: path + '/tests/1' })).toHaveProperty(
      'inputData',
      input,
    )
    api.state.user = { ...contestantUser }
    expect(() => api.handle({ method: 'GET', path: path + '/tests/1' })).toThrow()
  })
})
