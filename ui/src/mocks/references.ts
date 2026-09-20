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
    if (!/^[1-9]\d*$/.test(ref)) throw new MockError(404, '资源编号无效。')
    const item = catalog[kind].find((item) => item.publicId === ref)
    if (!item) throw new MockError(404, '演示资源不存在。')
    return item.id
  }
  const parts = request.path.split('/').filter(Boolean)
  const index = parts[1] === 'admin' || parts[1] === 'authoring' ? 2 : 1
  const kind = parts[index] as keyof typeof catalog
  if (Object.keys(catalog).includes(kind) && parts[index + 1])
    parts[index + 1] = resolve(kind, parts[index + 1])
  if (kind === 'contests' && parts[index + 2] === 'problems' && parts[index + 3]) {
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
  const body = request.body ? structuredClone(request.body) : undefined
  const references: Record<string, keyof typeof catalog> = {
    problemId: 'problems',
    problemIds: 'problems',
    contestId: 'contests',
    submissionIds: 'submissions',
  }
  const resolveBody = (value: unknown): void => {
    if (!value || typeof value !== 'object') return
    if (Array.isArray(value)) {
      value.forEach(resolveBody)
      return
    }
    for (const [key, field] of Object.entries(value)) {
      const kind = references[key]
      if (kind && field) {
        ;(value as Record<string, unknown>)[key] = Array.isArray(field)
          ? field.map((id) => resolve(kind, String(id)))
          : resolve(kind, String(field))
      } else resolveBody(field)
    }
  }
  resolveBody(body)
  return { ...request, path: '/' + parts.join('/'), params, body }
}

/** Fixtures use the same numeric resource identity as the browser API. */
export function normalizeResourceIdentities(state: MockState, response?: unknown) {
  const replacements = new Map<string, string>()
  for (const space of [state, ...Object.values(state.domainSpaces ?? {})])
    for (const items of Object.values(collections(space)))
      for (const item of items)
        if (item.publicId && item.id !== item.publicId) replacements.set(item.id, item.publicId)
  for (const domain of state.domains ?? [])
    for (const group of domain.groups ?? [])
      if (group.publicId && group.id !== group.publicId) replacements.set(group.id, group.publicId)
  if (!replacements.size) return
  const visited = new WeakSet<object>()
  const rewrite = (value: unknown): unknown => {
    if (typeof value === 'string') return replacements.get(value) ?? value
    if (!value || typeof value !== 'object' || visited.has(value)) return value
    visited.add(value)
    for (const [key, field] of Object.entries(value)) {
      let target = key
      if (key.includes('-'))
        target = key.replace(
          /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi,
          (id) => replacements.get(id) ?? id,
        )
      if (target !== key) delete (value as Record<string, unknown>)[key]
      ;(value as Record<string, unknown>)[target] = rewrite(field)
    }
    return value
  }
  rewrite(state)
  rewrite(response)
}

export function publicResponse(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(publicResponse)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(
    Object.entries(value)
      .filter(
        ([key]) =>
          !['publicId', 'problemPublicId', 'contestPublicId', 'sourceProblemId'].includes(key),
      )
      .map(([key, item]) => [key, publicResponse(item)]),
  )
}
