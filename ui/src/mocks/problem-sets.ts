import type { DtoSetItemRequest, DtoSetResponse } from '@/generated/api/model'
import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import { MockError } from './errors'
import { mockUsers } from './identities'
import { officialDomainID } from './problem-permissions'
import { allocateReference } from './references'
import { setPermissions } from './set-permissions'

export function problemSetRequest(
  state: MockState,
  { method, path, params = {}, body = {} }: MockRequest,
  now: number,
  problemVisible: (id: string) => boolean,
  progress: (id: string) => 'none' | 'attempted' | 'solved',
) {
  const [, , id, action, grantId] = path.split('/').filter(Boolean)
  const get = method === 'GET'
  const grants = (setId: string) => state.setGrants[setId] ?? []
  const caps = (set: DtoSetResponse) =>
    setPermissions(
      set,
      state.user,
      grants(set.id),
      set.items.every((p) => problemVisible(p.problemId)),
    )
  const view = (set: DtoSetResponse): DtoSetResponse => {
    if (!caps(set).view) throw new MockError(404, '题单不存在。')
    const items = set.items
      .filter((i) => problemVisible(i.problemId))
      .map((item) => {
        const problem = state.problems.find((p) => p.id === item.problemId)!
        return {
          ...item,
          title: problem.title,
          visibility: problem.visibility,
          tags: problem.tags,
          difficulty: problem.difficulty,
          userStatus: progress(item.problemId),
        }
      })
    return {
      ...set,
      items,
      permissions: caps(set),
      canEdit: caps(set).edit,
      problemCount: items.length,
      solvedCount: items.filter((p) => p.userStatus === 'solved').length,
    }
  }
  const input = () => {
    const title = String(body.title ?? '').trim()
    if (!title || title.length > 120) throw new MockError(400, '标题须为 1–120 个字符。')
    if (body.visibility && body.visibility !== 'public' && body.visibility !== 'private')
      throw new MockError(400, '无效可见性。')
    return {
      title,
      description: String(body.description ?? '').trim(),
      visibility: body.visibility === 'private' ? ('private' as const) : ('public' as const),
    }
  }
  if (!id && get) {
    const items = state.sets
      .filter(
        (s) =>
          caps(s).view &&
          (!params.author || s.authorId === params.author) &&
          `${s.title} ${s.description}`.includes(String(params.keyword ?? '')),
      )
      .map(view)
    const size = Math.max(1, Math.min(Number(params.size) || 20, 100)),
      page = Math.max(1, Number(params.page) || 1)
    return { items: items.slice((page - 1) * size, page * size), total: items.length }
  }
  if (!id && method === 'POST') {
    if (!state.user) throw new MockError(401, '请先登录。')
    const set: DtoSetResponse = {
      ...input(),
      id: crypto.randomUUID(),
      publicId: allocateReference(state, 'problem-sets'),
      domainId: officialDomainID,
      ownerId: state.user.id,
      ownerName: state.user.username,
      authorId: state.user.id,
      authorName: state.user.username,
      createdAt: new Date(now).toISOString(),
      updatedAt: new Date(now).toISOString(),
      items: [],
      problemCount: 0,
      solvedCount: 0,
      canEdit: true,
      permissions: setPermissions({ ownerId: state.user.id, visibility: 'private' }, state.user),
    }
    state.sets.unshift(set)
    return view(set)
  }
  const set = state.sets.find((s) => s.id === id)
  if (!set || !caps(set).view) throw new MockError(404, '题单不存在。')
  if (get && !action) return view(set)
  if (!state.user) throw new MockError(401, '请先登录。')
  const require = (allowed: boolean) => {
    if (!allowed) throw new MockError(403, '没有此题单的操作权限。')
  }
  if (action === 'access') {
    if (get) {
      require(caps(set).viewAccess)
      return { items: grants(id), total: grants(id).length }
    }
    require(caps(set).manageAccess)
    if (method === 'DELETE') {
      if (!grants(id).some((g) => String(g.id) === grantId))
        throw new MockError(404, '授权不存在。')
      state.setGrants[id] = grants(id).filter((g) => String(g.id) !== grantId)
      return { status: 'deleted' }
    }
    if (method === 'PUT') {
      if (!!body.username === !!body.group || !['reader', 'editor'].includes(String(body.role)))
        throw new MockError(400, '请选择一个用户或群组以及协作角色。')
      if (body.group) throw new MockError(400, '当前演示域中没有此群组。')
      const user = mockUsers.find((u) => u.username === String(body.username).trim())
      if (!user || user.id === set.ownerId)
        throw new MockError(400, '目标须为非 owner 的有效域成员。')
      const others = grants(id).filter((g) => g.userId !== user.id)
      const prior = grants(id).find((g) => g.userId === user.id)
      state.setGrants[id] = [
        ...others,
        {
          id: prior?.id ?? ++state.nextSetGrantId,
          userId: user.id,
          username: user.username,
          role: body.role as 'reader' | 'editor',
        },
      ]
      return { status: 'updated' }
    }
  }
  if (action === 'owner' && method === 'PUT') {
    require(caps(set).transfer)
    const user = mockUsers.find((u) => u.username === String(body.username ?? '').trim())
    if (!user) throw new MockError(400, '目标须为有效域成员。')
    set.ownerId = user.id
    set.ownerName = user.username
    state.setGrants[id] = grants(id).filter((g) => g.userId !== user.id)
    return { status: 'transferred' }
  }
  if (!action && method === 'DELETE') {
    require(caps(set).delete)
    state.sets = state.sets.filter((s) => s.id !== id)
    delete state.setGrants[id]
    return { status: 'deleted' }
  }
  if (method === 'PUT' && action === 'items') {
    require(caps(set).edit)
    if (!caps(set).editItems)
      throw new MockError(400, '存在不可见的已有条目，请先申请题目访问权限。')
    if (!Array.isArray(body.items) || body.items.length > 500)
      throw new MockError(400, '题单最多包含 500 道题。')
    const seen = new Set<string>()
    const items = (body.items as DtoSetItemRequest[]).map((item, index) => {
      if (seen.has(item.problemId)) throw new MockError(400, '题目不能重复。')
      seen.add(item.problemId)
      const p = state.problems.find((p) => p.id === item.problemId && p.domainId === set.domainId)
      if (!p || !problemVisible(p.id)) throw new MockError(400, '题目不可用。')
      if ((item.note ?? '').length > 500) throw new MockError(400, '备注不能超过 500 个字符。')
      return {
        problemId: p.id,
        problemPublicId: p.publicId,
        title: p.title,
        visibility: p.visibility,
        difficulty: p.difficulty,
        tags: p.tags,
        note: (item.note ?? '').trim(),
        sortOrder: index,
        userStatus: progress(p.id),
        acceptCount: p.acceptedCount,
        submitCount: p.submissionCount,
      }
    })
    set.items = items
  } else if (method === 'PUT' && !action) {
    require(caps(set).edit)
    const prepared = input()
    if (prepared.visibility !== set.visibility) require(caps(set).publish)
    Object.assign(set, prepared)
  } else throw new MockError(501, '此题单接口尚未提供 mock，未向真实后端发送请求。')
  set.updatedAt = new Date(now).toISOString()
  return view(set)
}
