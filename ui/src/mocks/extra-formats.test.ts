import { describe, expect, it } from 'vitest'
import { createFixtures, type MockState } from './fixtures'
import { createMockAPI } from './api'
import { rankboard } from './rankboard'
import { adminUser, demoUser } from './identities'
import type { DtoContestDetailsResponse } from '@/generated/api/model'

describe('additional format standings', () => {
  it.each([
    ['leduo', ['Wrong Answer', 'Accepted'], [0, 100], 950],
    ['leduo', ['Wrong Answer', 'Wrong Answer'], [80, 40], 800],
    ['cf', ['Wrong Answer', 'Accepted'], [0, 100], 830],
    ['cf', ['Compile Error', 'Accepted'], [0, 100], 880],
    ['cf', ['Wrong Answer', 'Wrong Answer'], [50, 90], 0],
    ['oi', ['Accepted', 'Compile Error'], [100, 0], 0],
  ] as const)(
    '%s scores official submission facts consistently',
    (format, statuses, scores, want) => {
      const now = Date.parse('2030-01-01T12:00:00Z')
      const state: MockState = createFixtures(now)
      state.user = { ...adminUser }
      const api = createMockAPI(state, () => now),
        contest = state.contests[0]
      contest.format = format
      contest.rule = format
      contest.beginAt = new Date(now - 60 * 60000).toISOString()
      contest.endAt = new Date(now + 60 * 60000).toISOString()
      contest.penalizeCompileError = false
      const details = api.handle({
        method: 'GET',
        path: `/api/contests/${contest.id}`,
      }) as DtoContestDetailsResponse
      const problem = { ...details.problems[0], points: 1000 }
      const template = state.submissions[0]
      state.registrations = { [demoUser.id]: [contest.id] }
      state.submissions = statuses.map((status, i) => ({
        ...template,
        id: String(i),
        contestId: contest.id,
        userId: demoUser.id,
        problemId: problem.problemId,
        status,
        score: scores[i],
        submittedAt: new Date(Date.parse(contest.beginAt) + [10, 30][i] * 60000).toISOString(),
      }))
      expect(rankboard(state, contest, [problem], true, true, now).rows[0].score).toBe(want)
    },
  )
})
