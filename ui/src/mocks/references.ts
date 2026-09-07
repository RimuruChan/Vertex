import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import { MockError } from './errors'

function collections(state: MockState) {
  return {
    problems: state.problems,
    contests: state.contests,
    submissions: state.submissions,
    editorials: state.editorials,
    'problem-sets': state.sets,
    announcements: state.announcements ?? [],
  }
}

export function initializeReferences(state: MockState) {
  state.nextPublicIds ??= {}
  state.contestProblemIds ??= {}
  for (const [kind, items] of Object.entries(collections(state))) {
    let next = Math.max(
      kind === 'problems' ? 1000 : 1,
      ...items.map((item) => Number(item.publicId || 0) + 1),
    )
    for (const item of items) item.publicId ||= String(next++)
    state.nextPublicIds[kind] = Math.max(state.nextPublicIds[kind] || 0, next)
  }
  for (const contest of state.contests)
    state.contestProblemIds[contest.id] ??= state.problems.slice(0, 6).map((p) => p.id)
  const problemRef = (id: string) => state.problems.find((p) => p.id === id)?.publicId || ''
  for (const submission of state.submissions) {
    submission.problemPublicId = problemRef(submission.problemId)
    submission.contestPublicId = state.contests.find((c) => c.id === submission.contestId)?.publicId
  }
  for (const editorial of state.editorials)
    editorial.problemPublicId = problemRef(editorial.problemId)
  for (const set of state.sets)
    for (const item of set.items) item.problemPublicId = problemRef(item.problemId)
}

export function allocateReference(state: MockState, kind: string) {
  return String(state.nextPublicIds[kind]++)
}

export function resolveMockRequest(state: MockState, request: MockRequest): MockRequest {
  const catalog = collections(state)
  const resolve = (kind: keyof typeof catalog, ref: string) => {
    if (!/^\d+$/.test(ref)) return ref
    const item = catalog[kind].find((item) => item.publicId === ref)
    if (!item) throw new MockError(404, '演示资源不存在。')
    return item.id
  }
  const parts = request.path.split('/').filter(Boolean)
  const index = parts[1] === 'admin' ? 2 : 1
  const kind = parts[index] as keyof typeof catalog
  if (Object.keys(catalog).includes(kind) && parts[index + 1])
    parts[index + 1] = resolve(kind, parts[index + 1])
  if (kind === 'contests' && parts[index + 2] === 'problems') {
    const ref = decodeURIComponent(parts[index + 3])
    if (ref.length <= 8 && !/^\d+$/.test(ref)) {
      const planned = state.contestEntries?.[parts[index + 1]]
      const problemId = planned
        ? planned.find((entry) => entry.label === ref)?.problemId
        : /^[A-F]$/.test(ref)
          ? state.contestProblemIds[parts[index + 1]]?.[ref.charCodeAt(0) - 65]
          : undefined
      if (!problemId) throw new MockError(404, '比赛题目不存在。')
      parts[index + 3] = problemId
    } else parts[index + 3] = resolve('problems', ref)
  }
  const params = { ...request.params }
  if (params.problem) params.problem = resolve('problems', String(params.problem))
  if (params.contest) params.contest = resolve('contests', String(params.contest))
  return { ...request, path: '/' + parts.join('/'), params }
}
