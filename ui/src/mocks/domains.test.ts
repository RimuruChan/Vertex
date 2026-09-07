import { describe, expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, demoUser, observerUser } from './identities'
import type {
  DtoDomainResponse,
  DtoProblemResponse,
  DtoWorkspaceResponse,
} from '@/generated/api/model'

describe('mock domain boundaries', () => {
  it('resolves the same public number separately and keeps old routes on the official domain', () => {
    const api = createMockAPI(createFixtures())
    const original = api.handle({ method: 'GET', path: '/api/problems/1000' }) as DtoProblemResponse
    const training = api.handle({
      method: 'GET',
      path: '/api/domains/training/problems/1000',
    }) as DtoProblemResponse
    expect(training.id).not.toBe(original.id)
    expect(training.domainId).not.toBe(original.domainId)
    expect(training.title).toContain('算法训练营')
    expect(api.handle({ method: 'GET', path: '/api/problems/1000' })).toHaveProperty(
      'id',
      original.id,
    )
    for (const [domain, id] of [
      ['official', training.id],
      ['training', original.id],
    ])
      expect(() =>
        api.handle({ method: 'GET', path: `/api/domains/${domain}/problems/${id}` }),
      ).toThrow()
    expect(() =>
      api.handle({
        method: 'POST',
        path: '/api/domains/training/submissions',
        body: { problemId: original.id, language: 'cpp', sourceCode: 'int main(){}' },
      }),
    ).toThrow()
  })
  it('keeps global identity intact while domain creation rights differ', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...demoUser }
    expect(() =>
      api.handle({ method: 'POST', path: '/api/admin/problems', body: { title: 'Denied' } }),
    ).toThrow()
    const created = api.handle({
      method: 'POST',
      path: '/api/domains/training/admin/problems',
      body: { title: 'Training draft', domainId: api.state.problems[0].domainId },
    }) as DtoProblemResponse
    expect(created.domainId).not.toBe(api.state.problems[0].domainId)
    expect(created.ownerId).toBe(demoUser.id)
    expect(api.state.user.role).toBe('user')
    const work = api.handle({
      method: 'GET',
      path: `/api/domains/training/admin/problems/${created.publicId}/package`,
    }) as DtoWorkspaceResponse
    expect(work.meta.title).toBe('Training draft')
    expect(api.state.problems.some((problem) => problem.id === created.id)).toBe(false)
  })
  it('filters private domains, rechecks suspended members and forbids global routes under a domain', () => {
    const api = createMockAPI(createFixtures())
    const list = api.handle({ method: 'GET', path: '/api/domains' }) as {
      items: DtoDomainResponse[]
    }
    expect(list.items.map((domain) => domain.slug)).toEqual(['official', 'training'])
    expect(() =>
      api.handle({ method: 'GET', path: '/api/domains/private-team/problems/1000' }),
    ).toThrow()
    api.state.user = { ...observerUser }
    expect(
      api.handle({ method: 'GET', path: '/api/domains/private-team/problems/1000' }),
    ).toHaveProperty('publicId', '1000')
    expect(() =>
      api.handle({ method: 'POST', path: '/api/domains/private-team/submissions', body: {} }),
    ).toThrow('提交权限')
    api.state.domains!.find((domain) => domain.slug === 'private-team')!.members[
      observerUser.id
    ].status = 'suspended'
    expect(() =>
      api.handle({ method: 'GET', path: '/api/domains/private-team/problems/1000' }),
    ).toThrow()
    api.state.user = { ...adminUser }
    expect(() => api.handle({ method: 'GET', path: '/api/domains/training/admin/users' })).toThrow()
    const domain = api.state.domains!.find((domain) => domain.slug === 'training')!
    domain.archived = true
    expect(() =>
      api.handle({
        method: 'POST',
        path: '/api/domains/training/admin/problems',
        body: { title: 'No' },
      }),
    ).toThrow('归档')
  })
  it('creates and composes a contest using only published problems in its own domain', () => {
    const api = createMockAPI(createFixtures())
    const path = '/api/domains/training/admin/contests'
    const created = api.handle({
      method: 'POST',
      path,
      body: {
        title: 'Training event',
        beginAt: '2030-01-01T00:00:00Z',
        endAt: '2030-01-02T00:00:00Z',
        ownerId: adminUser.id,
      },
    }) as { id: string; publicId: string; ownerId: string }
    expect(created.ownerId).toBe(demoUser.id)
    const problem = api.handle({
      method: 'GET',
      path: '/api/domains/training/problems/1000',
    }) as DtoProblemResponse
    api.handle({
      method: 'PUT',
      path: `${path}/${created.publicId}/problems`,
      body: { problems: [{ problemId: problem.id, label: 'A', points: 200 }] },
    })
    const details = api.handle({ method: 'GET', path: `${path}/${created.publicId}` }) as {
      problems: { problemId: string; version: number; points: number }[]
    }
    expect(details.problems).toEqual([
      expect.objectContaining({ problemId: problem.id, version: 1, points: 200 }),
    ])
    const other = api.state.problems[0]
    expect(() =>
      api.handle({
        method: 'PUT',
        path: `${path}/${created.publicId}/problems`,
        body: { problems: [{ problemId: other.id, label: 'A' }] },
      }),
    ).toThrow()
    expect(api.handle({ method: 'GET', path: `${path}/${created.publicId}` })).toEqual(details)
    api.state.user = { ...observerUser }
    expect(() =>
      api.handle({
        method: 'PUT',
        path: `${path}/${created.publicId}`,
        body: { title: 'Changed' },
      }),
    ).toThrow()
  })
})
