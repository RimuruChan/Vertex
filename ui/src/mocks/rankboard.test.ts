import { describe, expect, it } from 'vitest'
import type { DtoRankboardResponse } from '@/generated/api/model'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { contestantUser, juryUser, demoUser, adminUser } from './identities'

describe('derived mock standings', () => {
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
