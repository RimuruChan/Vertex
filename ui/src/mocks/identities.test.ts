import { describe, expect, it } from 'vitest'
import type { DtoContestDetailsResponse, DtoSubmissionResponse } from '@/generated/api/model'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, contestantUser, demoUser, juryUser, observerUser } from './identities'

describe('independent demo identities', () => {
  it('keeps private questions and directed jury messages out of other contestants views', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...contestantUser }
    api.handle({
      method: 'POST',
      path: '/api/contests/1/clarifications',
      body: { subject: 'Private question', body: 'Only my question' },
    })
    api.state.user = { ...demoUser }
    api.handle({ method: 'POST', path: '/api/contests/1/register' })
    expect(api.handle({ method: 'GET', path: '/api/contests/1/clarifications' })).toEqual({
      items: [],
      total: 0,
    })
    api.state.user = { ...juryUser }
    api.handle({
      method: 'POST',
      path: '/api/contests/1/clarifications/reply',
      body: { recipientId: demoUser.id, subject: 'For demo', body: 'Directed response' },
    })
    api.state.user = { ...contestantUser }
    const own = api.handle({ method: 'GET', path: '/api/contests/1/clarifications' }) as {
      items: { subject: string }[]
    }
    expect(own.items.map((item) => item.subject)).toEqual(['Private question'])
    api.state.user = { ...demoUser }
    const directed = api.handle({ method: 'GET', path: '/api/contests/1/clarifications' }) as {
      items: { subject: string }[]
    }
    expect(directed.items.map((item) => item.subject)).toEqual(['For demo'])
  })
  it('keeps registration and private submissions separate from other accounts', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...contestantUser }
    expect(api.handle({ method: 'GET', path: '/api/contests/1/registration' })).toEqual({
      registered: true,
    })
    const created = api.handle({
      method: 'POST',
      path: '/api/submissions',
      body: {
        problemId: api.state.problems[0].id,
        contestId: api.state.contests[0].id,
        language: 'cpp',
        sourceCode: '// demo',
      },
    }) as DtoSubmissionResponse
    api.state.user = { ...demoUser }
    expect(api.handle({ method: 'GET', path: '/api/contests/1/registration' })).toEqual({
      registered: false,
    })
    expect(() =>
      api.handle({ method: 'GET', path: `/api/submissions/${created.publicId}` }),
    ).toThrow()
    api.state.user = { ...juryUser }
    expect(
      api.handle({ method: 'GET', path: `/api/submissions/${created.publicId}` }),
    ).toMatchObject({ id: created.id })
  })
  it('grants jury and observer capabilities only within their assigned contest', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...juryUser }
    const scoped = api.handle({
      method: 'GET',
      path: '/api/contests/1',
    }) as DtoContestDetailsResponse
    expect(scoped.staffRole).toBe('jury')
    expect(
      (api.handle({ method: 'GET', path: '/api/contests/2' }) as DtoContestDetailsResponse)
        .staffRole,
    ).toBe('')
    expect(() =>
      api.handle({ method: 'GET', path: '/api/contests/2/rankboard', params: { view: 'jury' } }),
    ).toThrow()
    api.state.user = { ...observerUser }
    expect(
      api.handle({ method: 'GET', path: '/api/contests/1/rankboard', params: { view: 'jury' } }),
    ).toHaveProperty('juryView', true)
    expect(() =>
      api.handle({
        method: 'POST',
        path: '/api/contests/1/clarifications/reply',
        body: { subject: 'notice', body: 'text' },
      }),
    ).toThrow('裁判')
    api.state.user = { ...adminUser }
    expect(api.handle({ method: 'GET', path: '/api/admin/stats' })).toHaveProperty('users')
  })
})
