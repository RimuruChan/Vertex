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
  const spaces = [state, ...Object.values(state.domainSpaces ?? {})]
  const problems = spaces.flatMap((space) => space.problems),
    submissions = spaces.flatMap((space) => space.submissions),
    contests = spaces.flatMap((space) => space.contests)
  const recent = submissions.filter((s) => Date.parse(s.submittedAt) >= now - 86400000)
  if (resource === 'stats')
    return {
      activeWorkers: 0,
      deadJobs: 0,
      queuedJobs: spaces.reduce((total, space) => total + Object.keys(space.pending).length, 0),
      runningJobs: 0,
      contests: contests.length,
      editorials: spaces.reduce(
        (total, space) =>
          total + space.editorials.filter((item) => item.status === 'published').length,
        0,
      ),
      problemSets: spaces.reduce((total, space) => total + space.sets.length, 0),
      problems: problems.length,
      publicProblems: problems.filter((p) => p.visibility === 'public' && p.publishedVersion > 0)
        .length,
      runningContests: contests.filter(
        (c) => Date.parse(c.beginAt) <= now && Date.parse(c.endAt) > now,
      ).length,
      submissions: submissions.length,
      submissionsToday: recent.length,
      users: mockUsers.length,
      usersToday: 0,
      verdictBreakdown: [...new Set(recent.map((s) => s.status))].map((verdict) => ({
        verdict,
        count: recent.filter((s) => s.status === verdict).length,
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
          submissions
            .filter((s) => s.userId === u.id && s.status === 'Accepted')
            .map((s) => s.problemId),
        ).size,
        submissionCount: submissions.filter((s) => s.userId === u.id).length,
      }))
    const size = Number(params.size) || 20,
      page = Number(params.page) || 1
    return { items: items.slice((page - 1) * size, page * size), total: items.length }
  }
  throw new MockError(501, '此管理接口尚未提供 mock，未向真实后端发送请求。')
}
