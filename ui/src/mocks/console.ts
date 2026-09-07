import type { DtoAccountResponse, DtoStatsResponse } from '@/generated/api/model'
import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import { mockUsers } from './identities'
import { MockError } from './errors'

export function adminReadRequest(
  state: MockState,
  { path, params = {} }: MockRequest,
  now: number,
) {
  const resource = path.split('/')[3]
  if (resource === 'stats')
    return {
      activeWorkers: 0,
      deadJobs: 0,
      queuedJobs: Object.keys(state.pending).length,
      runningJobs: 0,
      contests: state.contests.length,
      editorials: state.editorials.length,
      problemSets: state.sets.length,
      problems: state.problems.length,
      publicProblems: state.problems.filter((p) => p.visibility === 'public').length,
      runningContests: state.contests.filter(
        (c) => Date.parse(c.beginAt) <= now && Date.parse(c.endAt) > now,
      ).length,
      submissions: state.submissions.length,
      submissionsToday: state.submissions.filter(
        (s) => s.submittedAt.slice(0, 10) === new Date(now).toISOString().slice(0, 10),
      ).length,
      users: mockUsers.length,
      usersToday: 0,
      verdictBreakdown: [...new Set(state.submissions.map((s) => s.status))].map((verdict) => ({
        verdict,
        count: state.submissions.filter((s) => s.status === verdict).length,
      })),
    } satisfies DtoStatsResponse
  if (resource === 'users') {
    const items: DtoAccountResponse[] = mockUsers
      .filter(
        (u) =>
          (!params.role || u.role === params.role) &&
          u.username.includes(String(params.keyword ?? '')),
      )
      .map((u) => ({
        ...u,
        role: u.role === 'admin' ? 'admin' : 'user',
        createdAt: state.problems[0].createdAt,
        disabled: false,
        rating: 0,
        solvedCount: new Set(
          state.submissions
            .filter((s) => s.userId === u.id && s.status === 'Accepted')
            .map((s) => s.problemId),
        ).size,
        submissionCount: state.submissions.filter((s) => s.userId === u.id).length,
      }))
    const size = Number(params.size) || 20,
      page = Number(params.page) || 1
    return { items: items.slice((page - 1) * size, page * size), total: items.length }
  }
  throw new MockError(501, '此管理接口尚未提供 mock，未向真实后端发送请求。')
}
