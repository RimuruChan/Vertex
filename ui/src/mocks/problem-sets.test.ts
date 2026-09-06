import { describe, expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, demoUser, juryUser, observerUser } from './identities'
import type { DtoSetResponse } from '@/generated/api/model'

describe('problem set ownership in mock mode', () => {
  const setup = () => {
    const api = createMockAPI(createFixtures())
    const set = api.state.sets[0]
    const path = `/api/problem-sets/${set.id}`
    api.handle({ method: 'PUT', path, body: { title: set.title, visibility: 'private' } })
    return { api, set, path }
  }
  it('uses collaboration capabilities instead of creator or site-role checks', () => {
    const { api, path } = setup()
    api.handle({
      method: 'PUT',
      path: `${path}/access`,
      body: { username: 'jury', role: 'editor' },
    })
    api.handle({
      method: 'PUT',
      path: `${path}/access`,
      body: { username: 'observer', role: 'reader' },
    })
    api.state.user = { ...juryUser }
    const edited = api.handle({
      method: 'PUT',
      path,
      body: { title: 'Edited', visibility: 'private' },
    }) as DtoSetResponse
    expect(edited.permissions).toMatchObject({
      edit: true,
      delete: false,
      publish: false,
      manageAccess: false,
    })
    expect(() =>
      api.handle({ method: 'PUT', path, body: { title: 'Publish', visibility: 'public' } }),
    ).toThrow('操作权限')
    expect(() => api.handle({ method: 'DELETE', path })).toThrow('操作权限')
    expect(() =>
      api.handle({
        method: 'PUT',
        path: `${path}/access`,
        body: { username: 'contestant', role: 'editor' },
      }),
    ).toThrow('操作权限')
    api.state.user = { ...observerUser }
    expect(api.handle({ method: 'GET', path })).toHaveProperty('permissions.edit', false)
    expect(() =>
      api.handle({ method: 'PUT', path, body: { title: 'Denied', visibility: 'private' } }),
    ).toThrow('操作权限')
    api.state.user = null
    expect(() => api.handle({ method: 'GET', path })).toThrow('不存在')
    api.state.user = { ...adminUser }
    expect(api.handle({ method: 'GET', path })).toHaveProperty('permissions.manageAccess', true)
  })
  it('transfers ownership without retaining creator privileges or the new owner grant', () => {
    const { api, set, path } = setup()
    api.handle({
      method: 'PUT',
      path: `${path}/access`,
      body: { username: 'jury', role: 'reader' },
    })
    api.handle({ method: 'PUT', path: `${path}/owner`, body: { username: 'jury' } })
    expect(set.authorId).toBe(demoUser.id)
    expect(set.ownerId).toBe(juryUser.id)
    expect(() => api.handle({ method: 'GET', path })).toThrow('不存在')
    api.state.user = { ...juryUser }
    expect(api.handle({ method: 'GET', path: `${path}/access` })).toEqual({ items: [], total: 0 })
    expect(api.handle({ method: 'GET', path })).toHaveProperty('permissions.transfer', true)
  })
  it('hides private target metadata and rejects destructive replacement of incomplete views', () => {
    const { api, set, path } = setup()
    const problem = api.state.problems.find((p) => p.id === set.items[0].problemId)!
    problem.ownerId = adminUser.id
    problem.visibility = 'private'
    problem.title = 'Private metadata'
    const before = set.items.length
    const viewed = api.handle({ method: 'GET', path }) as DtoSetResponse
    expect(viewed.items).toHaveLength(before - 1)
    expect(viewed.problemCount).toBe(before - 1)
    expect(viewed.permissions).toMatchObject({ edit: true, editItems: false })
    expect(JSON.stringify(viewed)).not.toContain('Private metadata')
    expect(() => api.handle({ method: 'PUT', path: `${path}/items`, body: { items: [] } })).toThrow(
      '不可见的已有条目',
    )
    expect(set.items).toHaveLength(before)
    api.state.user = { ...adminUser }
    expect(api.handle({ method: 'GET', path })).toHaveProperty('permissions.editItems', true)
    api.handle({ method: 'PUT', path: `${path}/items`, body: { items: [] } })
    expect(set.items).toHaveLength(0)
    expect(api.state.problems.some((p) => p.id === problem.id)).toBe(true)
  })
})
