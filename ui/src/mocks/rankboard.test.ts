import { describe, expect, it } from 'vitest'
import type { DtoRankboardResponse } from '@/generated/api/model'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { contestantUser, juryUser, demoUser, adminUser } from './identities'

describe('derived mock standings', () => {
  it.each(['icpc', 'cf', 'ioi', 'leduo'] as const)(
    'ignores post-freeze retries for an already solved %s problem',
    (format) => {
      const now = Date.parse('2030-01-01T12:00:00Z')
      const api = createMockAPI(createFixtures(now), () => now)
      const contest = api.state.contests[0]
      contest.format = format
      contest.beginAt = new Date(now - 3600000).toISOString()
      contest.endAt = new Date(now + 3600000).toISOString()
      contest.freezeAt = new Date(now - 20000).toISOString()
      contest.unfreezeAt = undefined
      api.state.user = { ...contestantUser }
      api.state.registrations = { [contestantUser.id]: [contest.id] }
      const sample = api.state.submissions[0]
      api.state.submissions = [30000, 10000].map((offset, index) => ({
        ...sample,
        id: `retry-${index}`,
        userId: contestantUser.id,
        contestId: contest.id,
        problemId: api.state.problems[0].id,
        submittedAt: new Date(now - offset).toISOString(),
        status: 'Accepted',
        score: 100,
      }))
      const board = () =>
        api.handle({
          method: 'GET',
          path: `/api/contests/${contest.id}/rankboard`,
        }) as DtoRankboardResponse
      expect(board().rows[0].hasPending).toBe(false)
      expect(board().rows[0].cells[0].pendingCount).toBe(0)
      // Overturn the visible acceptance: the later hidden AC must stay pending.
      api.state.submissions[0].status = 'Wrong Answer'
      api.state.submissions[0].score = 0
      expect(board().rows[0].hasPending).toBe(true)
      expect(board().rows[0].cells[0]).toMatchObject({ score: 0, pendingCount: 1 })
      expect(board().rows[0].cells[0].solvedAt).toBeUndefined()
    },
  )
  it('marks the explicitly requested view independently of public freeze state', () => {
    const now = Date.parse('2030-01-01T12:00:00Z')
    const api = createMockAPI(createFixtures(now), () => now)
    const contest = api.state.contests[0]
    for (const user of [adminUser, juryUser]) {
      api.state.user = { ...user }
      for (const phase of ['unfrozen', 'frozen', 'released']) {
        contest.freezeAt = phase === 'unfrozen' ? undefined : new Date(now - 20000).toISOString()
        contest.unfreezeAt = phase === 'released' ? new Date(now - 1).toISOString() : undefined
        for (const jury of [false, true]) {
          const board = api.handle({
            method: 'GET',
            path: `/api/contests/${contest.id}/rankboard`,
            params: jury ? { view: 'jury' } : {},
          }) as DtoRankboardResponse
          expect(board.juryView).toBe(jury)
          expect(board.frozen).toBe(phase === 'frozen' && !jury)
        }
      }
    }
  })
  it('uses registered entrants and reveals full cells on public unfreeze without jury privileges', () => {
    const now = Date.parse('2030-01-01T12:00:00Z'),
      api = createMockAPI(createFixtures(now), () => now),
      contest = api.state.contests[0],
      p = api.state.problems[0]
    api.state.user = { ...contestantUser }
    api.state.registrations = { [contestantUser.id]: [contest.id] }
    const sub = {
      ...api.state.submissions[0],
      userId: contestantUser.id,
      problemId: p.id,
      contestId: contest.id,
      status: 'Accepted',
      score: 100,
      submittedAt: new Date(now - 10000).toISOString(),
    }
    api.state.submissions = [sub]
    contest.freezeAt = new Date(now - 20000).toISOString()
    contest.unfreezeAt = undefined
    const board = (jury = false) =>
      api.handle({
        method: 'GET',
        path: `/api/contests/${contest.id}/rankboard`,
        params: jury ? { view: 'jury' } : {},
      }) as DtoRankboardResponse
    expect(board(true)).toMatchObject({
      frozen: true,
      juryView: false,
      rows: [{ userId: contestantUser.id, solved: 0 }],
    })
    expect(board().rows[0].cells[0]).toMatchObject({ score: 0, pendingCount: 1 })
    api.state.user = { ...juryUser }
    expect(board(true)).toMatchObject({ juryView: true, frozen: false, rows: [{ solved: 1 }] })
    api.state.user = { ...contestantUser }
    contest.unfreezeAt = new Date(now - 1).toISOString()
    expect(board()).toMatchObject({ juryView: false, frozen: false, rows: [{ solved: 1 }] })
    expect(board().rows[0].cells[0]).toMatchObject({
      score: 100,
      pendingCount: 0,
      firstSolver: true,
    })
  })
  it('does not show an unopened, password-restricted or hidden board to ordinary viewers', () => {
    const now = Date.parse('2030-01-01T12:00:00Z'),
      api = createMockAPI(createFixtures(now), () => now),
      contest = api.state.contests[0]
    api.state.user = { ...demoUser }
    api.state.registrations = {}
    const board = () => api.handle({ method: 'GET', path: `/api/contests/${contest.id}/rankboard` })
    contest.visibility = 'password'
    expect(board).toThrow()
    contest.visibility = 'public'
    contest.beginAt = new Date(now + 10000).toISOString()
    expect(board).toThrow()
    contest.beginAt = new Date(now - 10000).toISOString()
    contest.rankboardVisible = false
    expect(board).toThrow()
    api.state.user = { ...adminUser }
    expect(board()).toHaveProperty('rows', [])
  })
  it('computes IOI best and OI final scores from actual simulated submissions', () => {
    const now = Date.parse('2030-01-01T12:00:00Z'),
      api = createMockAPI(createFixtures(now), () => now),
      contest = api.state.contests[0],
      p = api.state.problems[0]
    api.state.user = { ...juryUser }
    api.state.registrations = { [contestantUser.id]: [contest.id] }
    const sample = api.state.submissions[0]
    api.state.submissions = [100, 40].map((score, i) => ({
      ...sample,
      id: crypto.randomUUID(),
      userId: contestantUser.id,
      problemId: p.id,
      contestId: contest.id,
      status: score === 100 ? 'Accepted' : 'Wrong Answer',
      score,
      submittedAt: new Date(now - 20000 + i * 10000).toISOString(),
    }))
    const board = () =>
      api.handle({
        method: 'GET',
        path: `/api/contests/${contest.id}/rankboard`,
        params: { view: 'jury' },
      }) as DtoRankboardResponse
    contest.format = 'ioi'
    expect(board()).toMatchObject({ format: 'ioi', rows: [{ score: 100, solved: 1 }] })
    contest.format = 'oi'
    expect(board()).toMatchObject({ format: 'oi', rows: [{ score: 40, solved: 0 }] })
  })
})
