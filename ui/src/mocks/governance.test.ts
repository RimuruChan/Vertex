import { describe, expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { demoUser, juryUser, observerUser, contestantUser, adminUser } from './identities'
import type { DtoDomainResponse, DtoGroupResponse } from '@/generated/api/model'

const setup = () => createMockAPI(createFixtures(), () => Date.UTC(2026, 8, 7))
describe('domain and group governance', () => {
  it('creates an empty private space owned by the caller and keeps the official domain intact', () => {
    const api = setup(),
      before = api.state.problems.length
    const space = api.handle({
      method: 'POST',
      path: '/api/domains',
      body: { slug: 'my-lab', name: 'My lab', ownerId: adminUser.id },
    }) as DtoDomainResponse
    expect(space.ownerId).toBe(demoUser.id)
    expect(space.visibility).toBe('private')
    expect(space.canArchive).toBe(true)
    for (const resource of [
      'problems',
      'contests',
      'submissions',
      'editorials',
      'problem-sets',
      'announcements',
    ])
      expect(api.handle({ method: 'GET', path: `/api/domains/my-lab/${resource}` })).toEqual({
        items: [],
        total: 0,
      })
    expect(api.state.problems).toHaveLength(before)
    expect(() =>
      api.handle({
        method: 'POST',
        path: '/api/domains',
        body: { slug: 'official', name: 'Hijack' },
      }),
    ).toThrow()
    api.state.user = { ...adminUser }
    expect(() =>
      api.handle({
        method: 'PUT',
        path: '/api/domains/official',
        body: { name: 'Official', visibility: 'private', joinPolicy: 'invite' },
      }),
    ).toThrow()
    expect(() =>
      api.handle({
        method: 'PUT',
        path: '/api/domains/official/archive',
        body: { archived: true },
      }),
    ).toThrow()
  })
  it('separates joining, approval, invitation and active membership', () => {
    const api = setup()
    api.handle({
      method: 'POST',
      path: '/api/domains',
      body: {
        slug: 'approval-lab',
        name: 'Approval',
        visibility: 'public',
        joinPolicy: 'approval',
      },
    })
    api.state.user = { ...contestantUser }
    expect(
      api.handle({ method: 'POST', path: '/api/domains/approval-lab/membership' }),
    ).toHaveProperty('memberStatus', 'pending')
    expect(() => api.handle({ method: 'GET', path: '/api/domains/approval-lab/members' })).toThrow()
    api.state.user = { ...demoUser }
    api.handle({
      method: 'PUT',
      path: '/api/domains/approval-lab/members/contestant',
      body: { roleKey: 'member', status: 'active' },
    })
    api.state.user = { ...contestantUser }
    expect(api.handle({ method: 'GET', path: '/api/domains/approval-lab' })).toHaveProperty(
      'memberStatus',
      'active',
    )
    api.state.user = { ...demoUser }
    api.handle({
      method: 'POST',
      path: '/api/domains',
      body: { slug: 'invited-lab', name: 'Invited' },
    })
    api.handle({
      method: 'PUT',
      path: '/api/domains/invited-lab/members/observer',
      body: { roleKey: 'viewer', status: 'invited' },
    })
    api.state.user = { ...observerUser }
    expect(api.handle({ method: 'GET', path: '/api/domains/invited-lab' })).toHaveProperty(
      'canEnter',
      false,
    )
    expect(
      api.handle({ method: 'POST', path: '/api/domains/invited-lab/membership' }),
    ).toMatchObject({ memberStatus: 'active', memberRole: 'viewer' })
  })
  it('limits delegation and protects built-in and assigned roles', () => {
    const api = setup()
    api.state.user = { ...juryUser }
    const roles = '/api/domains/training/roles'
    api.handle({
      method: 'PUT',
      path: roles + '/roles-only',
      body: { name: 'Roles only', permissions: ['domain.roles.manage'] },
    })
    api.handle({
      method: 'PUT',
      path: '/api/domains/training/members/observer',
      body: { roleKey: 'roles-only', status: 'active' },
    })
    api.state.user = { ...observerUser }
    expect(() =>
      api.handle({
        method: 'PUT',
        path: roles + '/escalated',
        body: { name: 'Too much', permissions: ['problem.create'] },
      }),
    ).toThrow('自己没有')
    expect(() =>
      api.handle({
        method: 'PUT',
        path: roles + '/site',
        body: { name: 'Site', permissions: ['site.admin'] },
      }),
    ).toThrow('不合法')
    expect(() => api.handle({ method: 'DELETE', path: roles + '/member' })).toThrow('内置')
    expect(() => api.handle({ method: 'DELETE', path: roles + '/roles-only' })).toThrow('成员使用')
    api.handle({ method: 'PUT', path: roles + '/empty', body: { name: 'Empty', permissions: [] } })
    api.handle({ method: 'DELETE', path: roles + '/empty' })
  })
  it('lets a group manager maintain members without transferring, deleting, or governing the domain', () => {
    const api = setup(),
      base = '/api/domains/training/groups/1'
    expect(api.handle({ method: 'GET', path: base })).toMatchObject({
      canManage: true,
      canTransfer: false,
      canDelete: false,
    })
    api.handle({ method: 'PUT', path: base + '/members/contestant', body: { role: 'member' } })
    expect(api.handle({ method: 'GET', path: base + '/members' })).toHaveProperty('total', 4)
    expect(() =>
      api.handle({ method: 'PUT', path: base + '/owner', body: { username: 'demo' } }),
    ).toThrow()
    expect(() => api.handle({ method: 'DELETE', path: base })).toThrow()
    expect(() =>
      api.handle({
        method: 'PUT',
        path: '/api/domains/training/members/contestant',
        body: { roleKey: 'admin', status: 'active' },
      }),
    ).toThrow()
    expect(() => api.handle({ method: 'DELETE', path: base + '/members/jury' })).toThrow('所有者')
    api.state.user = { ...observerUser }
    expect(api.handle({ method: 'GET', path: base })).toHaveProperty('canManage', false)
    expect(() => api.handle({ method: 'PUT', path: base, body: { name: 'No' } })).toThrow()
    api.state.user = { ...juryUser }
    api.handle({
      method: 'PUT',
      path: '/api/domains/training/members/demo',
      body: { roleKey: 'author', status: 'suspended' },
    })
    api.state.user = { ...demoUser }
    expect(() => api.handle({ method: 'GET', path: base })).toThrow('有效域成员')
  })
  it('rejects foreign group IDs and non-domain members, and never reuses group numbers', () => {
    const api = setup()
    api.state.user = { ...juryUser }
    const group = api.handle({
      method: 'POST',
      path: '/api/domains/training/groups',
      body: { name: 'Temporary' },
    }) as DtoGroupResponse
    expect(() =>
      api.handle({ method: 'GET', path: `/api/domains/private-team/groups/${group.id}` }),
    ).toThrow('不存在')
    expect(() =>
      api.handle({
        method: 'PUT',
        path: `/api/domains/training/groups/${group.publicId}/members/lin`,
        body: { role: 'member' },
      }),
    ).toThrow('同域')
    api.handle({ method: 'DELETE', path: `/api/domains/training/groups/${group.publicId}` })
    const next = api.handle({
      method: 'POST',
      path: '/api/domains/training/groups',
      body: { name: 'Next' },
    }) as DtoGroupResponse
    expect(Number(next.publicId)).toBeGreaterThan(Number(group.publicId))
  })
  it('protects the owner and preserves restoration authority after archiving', () => {
    const api = setup()
    api.state.user = { ...juryUser }
    expect(() =>
      api.handle({
        method: 'PUT',
        path: '/api/domains/training/members/jury',
        body: { roleKey: 'viewer', status: 'suspended' },
      }),
    ).toThrow('所有者')
    api.handle({ method: 'PUT', path: '/api/domains/training/archive', body: { archived: true } })
    expect(api.handle({ method: 'GET', path: '/api/domains/training' })).toMatchObject({
      canArchive: true,
      canTransfer: false,
      permissions: [],
    })
    expect(() =>
      api.handle({ method: 'POST', path: '/api/domains/training/groups', body: { name: 'No' } }),
    ).toThrow('归档')
    api.handle({ method: 'PUT', path: '/api/domains/training/archive', body: { archived: false } })
    api.handle({ method: 'PUT', path: '/api/domains/training/owner', body: { username: 'demo' } })
    expect(api.handle({ method: 'GET', path: '/api/domains/training' })).toHaveProperty(
      'canTransfer',
      false,
    )
  })
})
