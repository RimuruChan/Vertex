import type {
  DtoContentPermissions,
  DtoDiscussionResponse,
  DtoEditorialResponse,
} from '@/generated/api/model'
import type { MockState } from './fixtures'
import type { MockRequest } from './api'
import { MockError } from './errors'
import { officialDomainID, problemPermissions } from './problem-permissions'
import { allocateReference } from './references'
import { mockCan, mockManager } from './domain-policy'

export const noContentPermissions = (): DtoContentPermissions => ({
  view: false,
  viewBody: false,
  edit: false,
  delete: false,
  comment: false,
  vote: false,
})

export function contentRequest(
  state: MockState,
  { method, path, params = {}, body = {} }: MockRequest,
  now: number,
  progress: (id: string) => string,
) {
  const [, resource, id, action] = path.split('/').filter(Boolean)
  const get = method === 'GET',
    user = state.user
  const administrator = mockManager(state.scope, user)
  const domainID = state.scope?.id ?? officialDomainID
  const owns = (authorId?: string) => !!user && authorId === user.id
  const problem = (id: string) => state.problems.find((p) => p.id === id && p.domainId === domainID)
  const problemVisible = (id: string) => {
    const p = problem(id)
    return !!p && problemPermissions(p, user, state.scope, state.problemGrants?.[p.id]).view
  }
  const problemModerator = (id: string) => {
    const p = problem(id)
    return !!p && !!user && (administrator || p.ownerId === user.id)
  }
  const require = (allowed: boolean) => {
    if (!allowed) throw new MockError(403, '没有此内容的操作权限。')
  }
  const authenticated = () => {
    if (!user) throw new MockError(401, '请先登录。')
    return user
  }
  const required = (key: string, max: number) => {
    const value = typeof body[key] === 'string' ? body[key].trim() : ''
    if (!value || new TextEncoder().encode(value).length > max)
      throw new MockError(400, '内容为空或超出长度限制。')
    return value
  }
  const input = () => {
    if (body.status && !['draft', 'published'].includes(String(body.status)))
      throw new MockError(400, '无效发布状态。')
    if (body.visibility && !['public', 'private'].includes(String(body.visibility)))
      throw new MockError(400, '无效可见性。')
    return {
      title: required('title', 200),
      contentMd: required('contentMd', 200000),
      status: body.status === 'draft' ? ('draft' as const) : ('published' as const),
      visibility: body.visibility === 'private' ? ('private' as const) : ('public' as const),
      solvedOnly: body.solvedOnly === true,
    }
  }
  const editorialCaps = (e: DtoEditorialResponse): DtoContentPermissions => {
    if (e.domainId !== domainID || !problemVisible(e.problemId)) return noContentPermissions()
    const own = owns(e.authorId),
      moderate = problemModerator(e.problemId)
    const view =
      (e.visibility === 'public' && e.status === 'published') || own || administrator === true
    const viewBody =
      view && (!e.solvedOnly || own || moderate || progress(e.problemId) === 'solved')
    const participate =
      viewBody && mockCan(state.scope, user, 'content.create') && e.status === 'published'
    return {
      view,
      viewBody,
      edit: view && own && !state.scope?.archived,
      delete: view && (own || moderate) && !state.scope?.archived,
      comment: participate,
      vote: participate,
    }
  }
  const editorialView = (e: DtoEditorialResponse): DtoEditorialResponse => {
    const permissions = editorialCaps(e)
    if (!permissions.view) throw new MockError(404, '题解不存在。')
    const parent = problem(e.problemId)!
    return {
      ...e,
      domainId: parent.domainId,
      problemTitle: parent.title,
      permissions,
      canEdit: permissions.edit,
      locked: !permissions.viewBody,
      contentMd: permissions.viewBody ? e.contentMd : '',
      voted: !!user && (state.editorialVotes[e.id] ?? []).includes(user.id),
    }
  }
  const editorial = (id: string) => {
    const e = state.editorials.find((e) => e.id === id)
    if (!e) throw new MockError(404, '题解不存在。')
    editorialView(e)
    return e
  }
  const thread = (kind: 'problemId' | 'editorialId', target: string) => {
    if (kind === 'problemId') {
      if (!problemVisible(target)) throw new MockError(404, '题目不存在。')
      return {
        canPost: mockCan(state.scope, user, 'content.create'),
        moderator: problemModerator(target),
      }
    }
    const e = editorialView(editorial(target))
    if (e.locked) throw new MockError(403, '通过题目后可见正文与讨论。')
    return { canPost: e.permissions.comment, moderator: e.permissions.delete }
  }
  const postView = (p: DtoDiscussionResponse) => {
    if (p.domainId !== domainID || (!p.problemId && !p.editorialId))
      throw new MockError(404, '讨论不存在。')
    const access = thread(p.problemId ? 'problemId' : 'editorialId', p.problemId ?? p.editorialId!)
    return {
      ...p,
      domainId: domainID,
      permissions: {
        ...noContentPermissions(),
        view: true,
        viewBody: true,
        edit: owns(p.authorId) && !state.scope?.archived,
        delete: !state.scope?.archived && (owns(p.authorId) || access.moderator),
        comment: access.canPost,
      },
    }
  }
  const removePosts = (ids: Set<number>) => {
    let changed = true
    while (changed) {
      changed = false
      for (const post of state.discussions) {
        if (post.parentId && ids.has(post.parentId) && !ids.has(post.id)) {
          ids.add(post.id)
          changed = true
        }
      }
    }
    state.discussions = state.discussions.filter((p) => !ids.has(p.id))
  }
  if (resource === 'contests') throw new MockError(404, '比赛交流请使用澄清。')
  if (action === 'discussions') {
    const field = resource === 'problems' ? 'problemId' : 'editorialId'
    const access = thread(field, id)
    if (get) {
      const items = state.discussions.filter((p) => p[field] === id).map(postView)
      return { items, total: items.length, canPost: access.canPost }
    }
    if (method === 'POST') {
      const actor = authenticated()
      require(access.canPost)
      const parentId = typeof body.parentId === 'number' ? body.parentId : undefined
      if (
        parentId !== undefined &&
        (!Number.isSafeInteger(parentId) ||
          parentId <= 0 ||
          !state.discussions.some((p) => p.id === parentId && p[field] === id))
      )
        throw new MockError(400, '父评论不属于此讨论。')
      const post: DtoDiscussionResponse = {
        id: ++state.nextDiscussionId,
        [field]: id,
        parentId,
        domainId: domainID,
        contentMd: required('contentMd', 20000),
        authorId: actor.id,
        authorName: actor.username,
        createdAt: new Date(now).toISOString(),
        updatedAt: new Date(now).toISOString(),
        edited: false,
        permissions: noContentPermissions(),
      }
      state.discussions.push(post)
      return postView(post)
    }
  }
  if (resource === 'discussions') {
    authenticated()
    const post = state.discussions.find((p) => p.id === Number(id))
    if (!post) throw new MockError(404, '讨论不存在。')
    const visible = postView(post)
    if (method === 'DELETE') {
      require(visible.permissions.delete)
      removePosts(new Set([post.id]))
      return { status: 'deleted' }
    }
    if (method === 'PUT') {
      require(visible.permissions.edit)
      post.contentMd = required('contentMd', 20000)
      post.updatedAt = new Date(now).toISOString()
      post.edited = true
      return postView(post)
    }
  }
  if (resource === 'editorials') {
    if (get && !id) {
      const items = state.editorials
        .filter(
          (e) =>
            editorialCaps(e).view &&
            (!params.problem || params.problem === e.problemId) &&
            (!params.author || params.author === e.authorId) &&
            `${e.title} ${problem(e.problemId)?.title ?? ''}`.includes(
              String(params.keyword ?? ''),
            ),
        )
        .map((e) => {
          const { contentMd, ...summary } = editorialView(e)
          void contentMd
          return summary
        })
        .sort((a, b) =>
          params.sort === 'votes'
            ? b.voteCount - a.voteCount
            : b.createdAt.localeCompare(a.createdAt),
        )
      const size = Math.max(1, Math.min(Number(params.size) || 20, 100)),
        page = Math.max(1, Number(params.page) || 1)
      return { items: items.slice((page - 1) * size, page * size), total: items.length }
    }
    if (get) return editorialView(editorial(id))
    const actor = authenticated()
    if (method === 'POST' && !id) {
      require(mockCan(state.scope, user, 'content.create'))
      const parent = problem(String(body.problemId ?? ''))
      if (!parent || !problemVisible(parent.id)) throw new MockError(404, '题目不存在。')
      const e: DtoEditorialResponse = {
        ...input(),
        id: crypto.randomUUID(),
        publicId: allocateReference(state, 'editorials'),
        domainId: parent.domainId,
        problemId: parent.id,
        problemPublicId: parent.publicId,
        problemTitle: parent.title,
        authorId: actor.id,
        authorName: actor.username,
        canEdit: true,
        permissions: noContentPermissions(),
        locked: false,
        voteCount: 0,
        voted: false,
        createdAt: new Date(now).toISOString(),
        updatedAt: new Date(now).toISOString(),
      }
      state.editorials.unshift(e)
      return editorialView(e)
    }
    const e = editorial(id),
      visible = editorialView(e)
    if (method === 'POST' && action === 'vote') {
      require(visible.permissions.vote)
      const voters = state.editorialVotes[e.id] ?? []
      const voted = voters.includes(actor.id),
        up = body.up === true
      if (voted !== up) {
        e.voteCount += up ? 1 : -1
        state.editorialVotes[e.id] = up
          ? [...voters, actor.id]
          : voters.filter((id) => id !== actor.id)
      }
      return { voted: up, voteCount: e.voteCount }
    }
    if (method === 'PUT') {
      require(visible.permissions.edit)
      Object.assign(e, input(), { updatedAt: new Date(now).toISOString() })
      return editorialView(e)
    }
    if (method === 'DELETE') {
      require(visible.permissions.delete)
      removePosts(new Set(state.discussions.filter((p) => p.editorialId === e.id).map((p) => p.id)))
      state.editorials = state.editorials.filter((value) => value.id !== e.id)
      delete state.editorialVotes[e.id]
      return { status: 'deleted' }
    }
  }
  throw new MockError(501, '此内容接口尚未提供 mock，未向真实后端发送请求。')
}
