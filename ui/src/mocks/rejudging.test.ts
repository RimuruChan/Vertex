import { describe, expect, it } from 'vitest'
import type { DtoRejudgingResponse, DtoSubmissionResponse } from '@/generated/api/model'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { contestantUser, juryUser, observerUser } from './identities'

describe('contest-scoped mock rejudging', () => {
  const setup = () => {
    let clock = Date.UTC(2026, 8, 7, 12)
    const api = createMockAPI(createFixtures(clock), () => clock)
    api.state.user = { ...contestantUser }
    const contestId = api.state.contests[0].id
    const submission = api.handle({
      method: 'POST',
      path: '/api/submissions',
      body: {
        contestId,
        problemId: api.state.problems[0].id,
        language: 'cpp',
        sourceCode: 'int main() {}',
      },
    }) as DtoSubmissionResponse
    clock += 5000
    api.handle({ method: 'GET', path: `/api/submissions/${submission.id}` })
    api.state.user = { ...juryUser }
    return {
      api,
      contestId,
      submission,
      advance: (ms: number) => {
        clock += ms
      },
    }
  }
  it('lets jury queue and cancel before leasing while observers remain read-only', () => {
    const { api, contestId, submission } = setup()
    const batch = api.handle({
      method: 'POST',
      path: '/api/admin/rejudgings',
      body: { contestId, reason: 'fixture' },
    }) as DtoRejudgingResponse
    expect(batch.total).toBe(1)
    expect(api.handle({ method: 'GET', path: `/api/submissions/${submission.id}` })).toHaveProperty(
      'status',
      'Pending',
    )
    api.state.user = { ...observerUser }
    expect(api.handle({ method: 'GET', path: `/api/admin/rejudgings/${batch.id}` })).toHaveProperty(
      'total',
      1,
    )
    expect(() =>
      api.handle({ method: 'POST', path: `/api/admin/rejudgings/${batch.id}/cancel` }),
    ).toThrow('重测权限')
    api.state.user = { ...juryUser }
    api.handle({ method: 'POST', path: `/api/admin/rejudgings/${batch.id}/cancel` })
    expect(api.handle({ method: 'GET', path: `/api/submissions/${submission.id}` })).toMatchObject({
      status: 'Accepted',
      sourceCode: 'int main() {}',
    })
  })
  it('reports simulated changes without running code or crossing another contest', () => {
    const { api, contestId, submission, advance } = setup()
    expect(() =>
      api.handle({
        method: 'POST',
        path: '/api/admin/rejudgings',
        body: { contestId: api.state.contests[1].id },
      }),
    ).toThrow('重测权限')
    api.nextVerdict = 'Wrong Answer'
    const batch = api.handle({
      method: 'POST',
      path: '/api/admin/rejudgings',
      body: { contestId },
    }) as DtoRejudgingResponse
    advance(5000)
    expect(api.handle({ method: 'GET', path: `/api/admin/rejudgings/${batch.id}` })).toMatchObject({
      state: 'finished',
      done: 1,
      changed: 1,
    })
    const changes = api.handle({
      method: 'GET',
      path: `/api/admin/rejudgings/${batch.id}/changes`,
    }) as { items: { submissionId: string }[] }
    expect(changes.items[0].submissionId).toBe(submission.id)
    api.state.user = { ...contestantUser }
    expect(() => api.handle({ method: 'GET', path: `/api/admin/rejudgings/${batch.id}` })).toThrow(
      '不存在',
    )
  })
  it('ignores saved batches whose contest no longer exists', () => {
    const { api, contestId } = setup()
    const batch = api.handle({
      method: 'POST',
      path: '/api/admin/rejudgings',
      body: { contestId },
    }) as DtoRejudgingResponse
    api.state.contests = api.state.contests.filter((contest) => contest.id !== contestId)
    expect(api.handle({ method: 'GET', path: '/api/admin/rejudgings' })).toEqual({
      items: [],
      total: 0,
    })
    expect(() => api.handle({ method: 'GET', path: `/api/admin/rejudgings/${batch.id}` })).toThrow(
      '不存在',
    )
  })
})
