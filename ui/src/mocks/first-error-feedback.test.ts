import { expect, it } from 'vitest'
import { createFixtures } from './fixtures'
import { createMockAPI } from './api'
import { demoUser } from './identities'
import type { DtoSubmissionResponse } from '@/generated/api/model'

it('only exposes the first failed case for a contestant, including detail and progress reads', () => {
  const now = Date.parse('2030-01-01T12:00:00Z')
  const state = createFixtures(now)
  const contest = state.contests[0]
  contest.feedback = 'first_error'
  state.registrations[demoUser.id] = [contest.id]
  const submission = state.submissions[0]
  Object.assign(submission, {
    contestId: contest.id,
    userId: demoUser.id,
    status: 'Wrong Answer',
    caseResults: [
      {
        caseIndex: 5,
        verdict: 'Wrong Answer',
        timeMs: 400,
        memoryKb: 1024,
        checkerOutput: 'secret',
      },
      { caseIndex: 1, verdict: 'Accepted', timeMs: 10, memoryKb: 1024 },
      { caseIndex: 3, verdict: 'Time Limit Exceeded', timeMs: 1000, memoryKb: 1024 },
    ],
  })
  const api = createMockAPI(state, () => now)
  for (const suffix of ['', '/progress']) {
    const result = api.handle({
      method: 'GET',
      path: `/api/submissions/${submission.id}${suffix}`,
    }) as DtoSubmissionResponse
    expect(result.caseResults).toEqual([
      { caseIndex: 3, verdict: 'Time Limit Exceeded', timeMs: 0, memoryKb: 0 },
    ])
    expect(result.totalCases).toBe(0)
    expect(result.judgedCases).toBe(0)
    expect(result.totalTimeMs).toBe(0)
  }
})
