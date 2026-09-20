import type { DtoProblemResponse } from './models'
import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import { MockError } from './errors'
import { allocateReference } from './references'
import { officialDomainID, problemPermissions } from './problem-permissions'
import { mockCan } from './domain-policy'
import { updateGrant, removeGrant, visibleGrants, grantMember } from './resource-grants'

export function authoringRequest(
  state: MockState,
  { method, path, params = {}, body = {} }: MockRequest,
  now: number,
): unknown {
  const [, , resource, id, section, itemId] = path.split('/').filter(Boolean)
  const get = method === 'GET',
    post = method === 'POST',
    del = method === 'DELETE'
  const iso = new Date(now).toISOString()
  const text = (key: string) => (typeof body[key] === 'string' ? (body[key] as string) : '')
  const required = (key: string) => {
    const value = text(key).trim()
    if (!value) throw new MockError(400, '请填写必填内容。')
    return value
  }
  if (
    resource !== 'problems' ||
    (section && !['access', 'owner'].includes(section)) ||
    (!section && method === 'PUT')
  )
    throw new MockError(404, '出题接口不存在。')
  if (!id && get) {
    const items = state.problems.filter(
      (p) =>
        problemPermissions(p, state.user, state.scope, state.problemGrants?.[p.id]).readPackage &&
        p.title.includes(String(params.keyword ?? '')) &&
        (!params.visibility || p.visibility === params.visibility),
    )
    const size = Number(params.size) || 20,
      page = Number(params.page) || 1
    return {
      items: items.slice((page - 1) * size, page * size).map((p) => ({
        ...p,
        permissions: problemPermissions(p, state.user, state.scope, state.problemGrants?.[p.id]),
      })),
      total: items.length,
    }
  }
  if (!id && post) {
    if (!state.user) throw new MockError(401, '请先登录。')
    if (!mockCan(state.scope, state.user, 'problem.create'))
      throw new MockError(403, '当前域没有创建题目权限。')
    const problem: DtoProblemResponse = {
      publishedVersion: 0,
      ownerId: state.user.id,
      ownerName: state.user.username,
      domainId: state.scope?.id ?? officialDomainID,
      permissions: problemPermissions(
        { ownerId: state.user.id, visibility: text('visibility') || 'private' },
        state.user,
      ),
      publicId: allocateReference(state, 'problems'),
      id: crypto.randomUUID(),
      title: required('title'),
      statementMd: text('statementMd'),
      difficulty: Number(body.difficulty) || 1,
      source: text('source'),
      timeLimitMs: Number(body.timeLimitMs) || 1000,
      memoryLimitKb: Number(body.memoryLimitKb) || 262144,
      visibility: text('visibility') || 'private',
      judgeType: 'normal',
      tags: Array.isArray(body.tags) ? (body.tags as string[]) : [],
      authorId: state.user?.id,
      userStatus: 'none',
      acceptedCount: 0,
      submissionCount: 0,
      solvedUserCount: 0,
      createdAt: iso,
      updatedAt: iso,
    }
    state.problems.unshift(problem)
    return problem
  }
  const problem = state.problems.find((p) => p.id === id)
  if (!problem) throw new MockError(404, '演示题目不存在。')
  problem.permissions = problemPermissions(
    problem,
    state.user,
    state.scope,
    state.problemGrants?.[id],
  )
  if (!problem.permissions.readPackage)
    throw new MockError(get ? 404 : 403, '没有此题目的协作权限。')
  if (section === 'access') {
    const grants = visibleGrants(state.problemGrants?.[id] ?? [], state.scope)
    if (get) return { items: grants, total: grants.length }
    if (!problem.permissions.manageAccess) throw new MockError(403, '没有管理协作权限')
    state.problemGrants ??= {}
    if (method === 'PUT')
      state.problemGrants[id] = updateGrant(
        state,
        grants,
        problem.ownerId,
        body,
        ['reader', 'editor'],
        () => (state.nextProblemGrantId = (state.nextProblemGrantId ?? 0) + 1),
      )
    else if (del) state.problemGrants[id] = removeGrant(grants, itemId)
    else throw new MockError(404, '授权接口不存在')
    return { status: 'updated' }
  }
  if (section === 'owner' && method === 'PUT') {
    if (!problem.permissions.transfer) throw new MockError(403, '没有转让权限')
    const owner = grantMember(state, text('username'))
    problem.ownerId = owner.id
    problem.ownerName = owner.username
    if (state.problemGrants?.[id])
      state.problemGrants[id] = state.problemGrants[id].filter((grant) => grant.userId !== owner.id)
    return { status: 'transferred' }
  }
  if (!section && get) return problem
  if (!section && del) {
    if (!problem.permissions.delete) throw new MockError(403, '没有删除权限。')
    if (
      state.submissions.some((s) => s.problemId === id) ||
      Object.values(state.contestProblemIds).some((items) => items.includes(id))
    )
      throw new MockError(409, '已有比赛或提交引用，请隐藏题目。')
    state.problems = state.problems.filter((p) => p.id !== id)
    delete state.problemReleases[id]
    if (state.workbenches) delete state.workbenches[id]
    if (state.problemOrigins) delete state.problemOrigins[id]
    if (state.problemGrants) delete state.problemGrants[id]
    return { status: 'ok' }
  }
  throw new MockError(404, '出题接口不存在。')
}
