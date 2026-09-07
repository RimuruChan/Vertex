import { describe, expect, it } from 'vitest'
import type {
  DtoProblemResponse,
  DtoProfileResponse,
  DtoSubmissionResponse,
} from '@/generated/api/model'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, demoUser, contestantUser, juryUser } from './identities'

describe('aggregate and feedback read policies', () => {
  it('uses all domain data graphs for authorized site administration', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...adminUser }
    const stats = api.handle({ method: 'GET', path: '/api/admin/stats' }) as {
      problems: number
      submissions: number
    }
    const spaces = [api.state, ...Object.values(api.state.domainSpaces ?? {})]
    expect(stats.problems).toBeGreaterThan(api.state.problems.length)
    expect(stats.problems).toBe(spaces.reduce((n, space) => n + space.problems.length, 0))
    expect(stats.submissions).toBe(spaces.reduce((n, space) => n + space.submissions.length, 0))
    api.state.user = { ...juryUser }
    expect(() => api.handle({ method: 'GET', path: '/api/admin/stats' })).toThrow('站点管理员')
    expect(() => api.handle({ method: 'GET', path: '/api/domains/training/admin/stats' })).toThrow()
  })
  it('keeps private and unpublished problems out of public profile totals', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...adminUser }
    const [a, b, c] = api.state.problems
    api.state.problems = [a, b, c]
    a.visibility = 'public'
    a.difficulty = 2
    b.visibility = 'private'
    b.difficulty = 5
    c.visibility = 'public'
    c.publishedVersion = 0
    c.difficulty = 9
    const sample = api.state.submissions[0]
    api.state.submissions = [a, b].map((p) => ({
      ...sample,
      id: crypto.randomUUID(),
      userId: demoUser.id,
      problemId: p.id,
      contestId: undefined,
      status: 'Accepted',
    }))
    const profile = api.handle({ method: 'GET', path: '/api/users/demo' }) as DtoProfileResponse
    expect(profile.submissionCount).toBe(1)
    expect(profile.solvedCount).toBe(1)
    expect(profile.byDifficulty).toEqual([{ difficulty: 2, solved: 1, total: 1 }])
  })
  it('offers private published reuse candidates only to their collaborators', () => {
    const api = createMockAPI(createFixtures()),
      p = api.state.problems[0]
    p.visibility = 'private'
    const list = (view?: string) =>
      (
        api.handle({ method: 'GET', path: '/api/problems', params: { view, size: 100 } }) as {
          items: DtoProblemResponse[]
        }
      ).items
    expect(list('available').some((item) => item.id === p.id)).toBe(false)
    api.state.user = { ...adminUser }
    api.handle({
      method: 'PUT',
      path: `/api/admin/problems/${p.id}/access`,
      body: { username: 'demo', role: 'reader' },
    })
    api.state.user = { ...demoUser }
    expect(list('available').some((item) => item.id === p.id)).toBe(true)
    expect(list().some((item) => item.id === p.id)).toBe(false)
    api.state.user = null
    expect(() => list('available')).toThrow()
  })
  it('redacts details, progress and status-filter totals without altering the stored verdict', () => {
    let now = Date.parse('2030-01-01T12:00:00Z')
    const api = createMockAPI(createFixtures(now), () => now)
    api.state.user = { ...contestantUser }
    api.state.submissions = []
    const event = api.state.contests[0]
    event.feedback = 'none'
    const sub = api.handle({
      method: 'POST',
      path: '/api/submissions',
      body: {
        problemId: api.state.problems[0].id,
        contestId: event.id,
        language: 'cpp',
        sourceCode: 'private fixture',
      },
    }) as DtoSubmissionResponse
    now += 5000
    const detail = api.handle({
      method: 'GET',
      path: `/api/submissions/${sub.id}`,
    }) as DtoSubmissionResponse
    expect(detail).toMatchObject({
      status: 'Submitted',
      score: 0,
      caseResults: [],
      judgedCases: 0,
      totalCases: 0,
      sourceCode: 'private fixture',
    })
    expect(api.state.submissions[0].status).toBe('Accepted')
    const list = (status: string) =>
      api.handle({
        method: 'GET',
        path: '/api/submissions',
        params: { contest: event.id, status },
      }) as { items: DtoSubmissionResponse[]; total: number }
    expect(list('Accepted').total).toBe(0)
    expect(list('Submitted').total).toBe(1)
    expect(list('Submitted').items[0].sourceCode).toBeUndefined()
    expect(
      api.handle({ method: 'GET', path: `/api/submissions/${sub.id}/progress` }),
    ).toMatchObject({ status: 'Submitted', sourceCode: undefined })
    api.state.user = { ...juryUser }
    expect(list('Accepted').total).toBe(1)
    api.state.user = { ...contestantUser }
    now = Date.parse(event.endAt) + 1
    expect(list('Accepted').total).toBe(1)
  })
  it('does not reveal others results through private or still-frozen completed contests', () => {
    let now = Date.parse('2030-01-01T12:00:00Z')
    const api = createMockAPI(createFixtures(now), () => now)
    const event = api.state.contests[0]
    event.visibility = 'private'
    event.endAt = new Date(now - 1).toISOString()
    api.state.user = { ...demoUser }
    const rows = () =>
      api.handle({
        method: 'GET',
        path: '/api/submissions',
        params: { contest: event.id, user: 'contestant' },
      }) as { total: number }
    expect(rows().total).toBe(0)
    event.visibility = 'public'
    event.freezeAt = new Date(now - 10000).toISOString()
    event.unfreezeAt = undefined
    expect(rows().total).toBe(0)
  })
  it('updates only completed practice counters and counts each newly solved user once', () => {
    let now = Date.parse('2030-01-01T12:00:00Z')
    const api = createMockAPI(createFixtures(now), () => now)
    api.state.user = { ...contestantUser }
    api.state.submissions = []
    const problem = api.state.problems[0],
      before = {
        count: problem.submissionCount,
        ac: problem.acceptedCount,
        users: problem.solvedUserCount,
      }
    const submit = (contestId?: string) =>
      api.handle({
        method: 'POST',
        path: '/api/submissions',
        body: { problemId: problem.id, contestId, language: 'cpp', sourceCode: 'fixture' },
      }) as DtoSubmissionResponse
    const finish = (sub: DtoSubmissionResponse) => {
      now += 5000
      api.handle({ method: 'GET', path: `/api/submissions/${sub.id}` })
    }
    finish(submit(api.state.contests[0].id))
    expect(problem.submissionCount).toBe(before.count)
    const first = submit()
    expect(problem.submissionCount).toBe(before.count)
    finish(first)
    finish(submit())
    expect(problem.submissionCount).toBe(before.count + 2)
    expect(problem.acceptedCount).toBe(before.ac + 2)
    expect(problem.solvedUserCount).toBe(before.users + 1)
  })
})
