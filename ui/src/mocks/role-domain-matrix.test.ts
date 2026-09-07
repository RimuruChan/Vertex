import { describe, expect, it } from 'vitest'
import type { DtoDomainResponse, DtoProblemResponse, DtoUserResponse } from '@/generated/api/model'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, contestantUser, demoUser, juryUser, observerUser } from './identities'

describe('combined account and domain matrix', () => {
  it.each<{
    name: string
    user: DtoUserResponse | null
    create: string[]
    packages: string[]
    privateAccess: boolean
  }>([
    { name: 'guest', user: null, create: [], packages: [], privateAccess: false },
    {
      name: 'member',
      user: demoUser,
      create: ['training'],
      packages: ['training'],
      privateAccess: false,
    },
    { name: 'contestant', user: contestantUser, create: [], packages: [], privateAccess: false },
    {
      name: 'jury and domain owner',
      user: juryUser,
      create: ['training', 'private-team'],
      packages: ['training', 'private-team'],
      privateAccess: true,
    },
    {
      name: 'observer and readonly member',
      user: observerUser,
      create: [],
      packages: [],
      privateAccess: true,
    },
    {
      name: 'site administrator',
      user: adminUser,
      create: ['official', 'training', 'private-team'],
      packages: ['official', 'training', 'private-team'],
      privateAccess: true,
    },
  ])(
    '$name keeps independent domain capabilities and resource boundaries',
    ({ user, create, packages, privateAccess }) => {
      const api = createMockAPI(createFixtures())
      api.state.user = user ? { ...user } : null
      const originalIdentity = structuredClone(api.state.user)
      const seenIDs = new Set<string>()
      for (const slug of ['official', 'training', 'private-team']) {
        const path = `/api/domains/${slug}`
        if (slug === 'private-team' && !privateAccess) {
          expect(() => api.handle({ method: 'GET', path })).toThrow()
          expect(() => api.handle({ method: 'GET', path: `${path}/problems/1000` })).toThrow()
          continue
        }
        const domain = api.handle({ method: 'GET', path }) as DtoDomainResponse
        expect(domain.canEnter).toBe(true)
        expect(domain.permissions.includes('problem.create')).toBe(create.includes(slug))
        const problem = api.handle({
          method: 'GET',
          path: `${path}/problems/1000`,
        }) as DtoProblemResponse
        expect(problem.publicId).toBe('1000')
        expect(problem.domainId).toBe(domain.id)
        expect(problem.permissions.readPackage).toBe(packages.includes(slug))
        for (const foreignID of seenIDs) {
          expect(() =>
            api.handle({ method: 'GET', path: `${path}/problems/${foreignID}` }),
          ).toThrow()
        }
        seenIDs.add(problem.id)
        if (!packages.includes(slug)) {
          expect(() =>
            api.handle({ method: 'GET', path: `${path}/admin/problems/1000/package` }),
          ).toThrow()
        }
      }
      expect(api.state.user).toEqual(originalIdentity)
      const official = api.handle({
        method: 'GET',
        path: '/api/problems/1000',
      }) as DtoProblemResponse
      expect(official.domainId).toBe(api.state.problems[0].domainId)
    },
  )
})
