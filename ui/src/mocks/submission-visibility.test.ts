import { describe, expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { contestantUser, demoUser, observerUser } from './identities'

describe('peer submission policies', () => {
  it.each(['full', 'first_error'] as const)(
    'keeps source-bearing compiler output private at %s feedback',
    (feedback) => {
      const now = Date.parse('2030-01-01T12:00:00Z')
      const state = createFixtures(now)
      const contest = state.contests[0]
      Object.assign(contest, {
        feedback,
        submissionVisibility: 'during',
        sourceCodeVisibility: 'own',
      })
      const item = {
        ...state.submissions[0],
        contestId: contest.id,
        userId: contestantUser.id,
        status: 'Compile Error',
        sourceCode: 'PRIVATE_SOURCE_SENTINEL',
        compileResult: 'compiler: PRIVATE_SOURCE_SENTINEL',
      }
      state.submissions = [item]
      const api = createMockAPI(state, () => now)
      for (const suffix of ['', '/progress']) {
        state.user = { ...demoUser }
        const peer = JSON.stringify(
          api.handle({ method: 'GET', path: `/api/submissions/${item.id}${suffix}` }),
        )
        expect(peer).not.toContain('PRIVATE_SOURCE_SENTINEL')
        expect(peer).not.toContain('compileResult')
        state.user = { ...contestantUser }
        expect(
          api.handle({ method: 'GET', path: `/api/submissions/${item.id}${suffix}` }),
        ).toMatchObject({ compileResult: 'compiler: PRIVATE_SOURCE_SENTINEL' })
      }
    },
  )
  it('defaults to own records and applies frozen Pending before filtering and source projection', () => {
    let now = Date.parse('2030-01-01T12:00:00Z')
    const state = createFixtures(now)
    const contest = state.contests[0]
    contest.beginAt = new Date(now - 3600000).toISOString()
    contest.endAt = new Date(now + 3600000).toISOString()
    contest.feedback = 'full'
    state.user = { ...contestantUser }
    const api = createMockAPI(state, () => now)
    const created = api.handle({
      method: 'POST',
      path: '/api/submissions',
      body: {
        contestId: contest.id,
        problemId: state.contestProblemIds[contest.id][0],
        language: 'cpp',
        sourceCode: 'private code',
      },
    }) as { id: string }
    now += 6000
    const get = () => api.handle({ method: 'GET', path: `/api/submissions/${created.id}` })
    state.user = { ...demoUser }
    expect(get).toThrow()
    contest.submissionVisibility = 'during'
    contest.sourceCodeVisibility = 'after_end'
    expect(get()).toMatchObject({ status: 'Accepted', sourceCode: undefined })
    contest.freezeAt = new Date(now - 7000).toISOString()
    contest.frozenSubmissionVisibility = 'pending'
    expect(get()).toMatchObject({
      status: 'Pending',
      score: 0,
      totalTimeMs: 0,
      sourceCode: undefined,
      judgedAt: undefined,
      caseResults: [],
    })
    expect(
      api.handle({
        method: 'GET',
        path: '/api/submissions',
        params: { contest: contest.id, status: 'Accepted' },
      }),
    ).toMatchObject({ total: 0 })
    expect(
      api.handle({
        method: 'GET',
        path: '/api/submissions',
        params: { contest: contest.id, status: 'Pending' },
      }),
    ).toMatchObject({ total: 1 })
    state.user = { ...observerUser }
    expect(get()).toMatchObject({ status: 'Accepted', sourceCode: 'private code' })
    state.user = { ...demoUser }
    contest.frozenSubmissionVisibility = 'hidden'
    expect(get).toThrow()
    contest.endAt = new Date(now - 2).toISOString()
    contest.unfreezeAt = new Date(now - 1).toISOString()
    expect(get()).toMatchObject({ status: 'Accepted', sourceCode: 'private code' })
    contest.sourceCodeVisibility = 'own'
    expect(get()).toMatchObject({ sourceCode: undefined })
  })
})
