import type {
  DtoAuthResponse,
  DtoContestProblemResponse,
  DtoDiscussionResponse,
  DtoEditorialResponse,
  DtoProfileResponse,
  DtoRankboardResponse,
  DtoSetItemRequest,
  DtoSetResponse,
  DtoSubmissionResponse,
} from '@/generated/api/model'
import { createFixtures, demoUser, type MockState } from './fixtures'

export class MockError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

export type MockRequest = {
  method: string
  path: string
  params?: Record<string, unknown>
  body?: Record<string, unknown>
}
export type MockScenario = 'normal' | 'slow' | 'empty' | 'error'
export const demoPassword = 'demo123'

/** A small stateful API for UI development, not a judge or an authorization simulator. */
export function createMockAPI(state: MockState = createFixtures(), clock = Date.now) {
  let scenario: MockScenario = 'normal'
  let nextVerdict = 'Accepted'
  const isoNow = () => new Date(clock()).toISOString()
  function requireUser() {
    if (!state.user) throw new MockError(401, '请先登录演示账号。')
    return state.user
  }
  function found<T>(value: T | undefined): T {
    if (!value) throw new MockError(404, '演示数据中没有找到这条记录。')
    return value
  }
  function own(authorId: string | undefined) {
    if (authorId !== requireUser().id) throw new MockError(403, '只能编辑自己的演示内容。')
  }
  function progress(problemId: string): 'solved' | 'attempted' | 'none' {
    const attempts = state.submissions.filter(
      (s) => s.problemId === problemId && s.userId === state.user?.id,
    )
    return attempts.some((s) => s.status === 'Accepted')
      ? 'solved'
      : attempts.length
        ? 'attempted'
        : 'none'
  }
  function setView(set: DtoSetResponse): DtoSetResponse {
    const items = set.items.map((item) => ({ ...item, userStatus: progress(item.problemId) }))
    return {
      ...set,
      items,
      canEdit: state.user?.id === set.authorId,
      problemCount: items.length,
      solvedCount: items.filter((i) => i.userStatus === 'solved').length,
    }
  }
  function editorialView(editorial: DtoEditorialResponse): DtoEditorialResponse {
    const canEdit = state.user?.id === editorial.authorId
    const locked = !canEdit && editorial.solvedOnly && progress(editorial.problemId) !== 'solved'
    return {
      ...editorial,
      canEdit,
      locked,
      contentMd: locked ? '' : editorial.contentMd,
      voted: !!state.user && editorial.voted,
    }
  }
  function advanceSubmissions() {
    for (const [id, job] of Object.entries(state.pending)) {
      const submission = state.submissions.find((s) => s.id === id)
      if (!submission) {
        delete state.pending[id]
        continue
      }
      const elapsed = clock() - job.started
      submission.status = elapsed < 900 ? 'Pending' : elapsed < 4500 ? 'Judging' : job.verdict
      const judged = elapsed < 900 ? 0 : Math.min(5, Math.floor((elapsed - 900) / 720))
      submission.judgedCases = judged
      submission.totalTimeMs = judged * 7
      submission.peakMemoryKb = judged ? 3456 : 0
      submission.caseResults = Array.from({ length: judged }, (_, i) => ({
        caseIndex: i + 1,
        verdict: i === 4 ? job.verdict : 'Accepted',
        timeMs: 7,
        memoryKb: 3456,
      }))
      if (elapsed >= 4500) {
        submission.judgedCases = 5
        submission.score = job.verdict === 'Accepted' ? 100 : 0
        submission.judgedAt = isoNow()
        if (job.verdict === 'Compile Error') {
          submission.compileResult = 'demo.cpp:4:1: error: expected semicolon (模拟编译错误)'
          submission.caseResults = []
          submission.judgedCases = 0
        }
        const problem = state.problems.find((p) => p.id === submission.problemId)
        if (problem && job.verdict === 'Accepted') problem.acceptedCount++
        delete state.pending[id]
      }
    }
  }

  function route({ method, path, params = {}, body = {} }: MockRequest): unknown {
    advanceSubmissions()
    const parts = path.split('/').filter(Boolean).map(decodeURIComponent)
    const [, resource, id, action, childId] = parts
    const get = method.toUpperCase() === 'GET'
    const post = method.toUpperCase() === 'POST'
    const del = method.toUpperCase() === 'DELETE'
    const text = (key: string) =>
      typeof body[key] === 'string' ? (body[key] as string).trim() : ''
    const required = (key: string) => {
      const value = text(key)
      if (!value) throw new MockError(400, '请填写必填内容。')
      return value
    }
    const list = <T>(items: T[]) => {
      if (scenario === 'empty') return { items: [], total: 0 }
      const page = Math.max(1, Number(params.page) || 1)
      const size = Math.max(1, Number(params.size ?? params.limit) || 20)
      return { items: items.slice((page - 1) * size, page * size), total: items.length }
    }
    if (parts[0] !== 'api' || resource === 'admin' || parts.length > 5)
      throw new MockError(501, '此接口尚未提供 mock，未向真实后端发送请求。')
    if (resource === 'auth') {
      if (post && id === 'login') {
        if (text('username') !== demoUser.username || text('password') !== demoPassword)
          throw new MockError(401, '演示账号：demo，密码：demo123。')
        state.user = { ...demoUser }
      } else if (post && id === 'register') {
        throw new MockError(422, '演示模式不创建真实账号，请使用 demo / demo123 登录。')
      } else if (post && (id === 'logout' || id === 'logout-all')) {
        state.user = null
        return undefined
      } else if (!(get && id === 'me') && !(post && id === 'refresh')) {
        throw new MockError(404, '未知演示接口。')
      }
      const user = requireUser()
      if (id === 'me') return user
      return {
        accessToken: 'vertex-mock-only',
        token: 'vertex-mock-only',
        expiresIn: 3600,
        user,
      } satisfies DtoAuthResponse
    }
    if (scenario === 'error' && get)
      throw new MockError(503, '模拟加载失败。可在「演示模式」中切回正常，再点击重试。')
    if (resource === 'health' && get) return { status: 'ok' }
    if (resource === 'announcements' && get)
      return list([
        {
          id: 'mock-welcome',
          title: '周末练习赛开放报名',
          contentMd: '选一个安静的下午，一起解几道题。比赛期间可在澄清区提问。',
          pinned: true,
          published: true,
          authorName: 'Vertex',
          createdAt: state.contests[0].createdAt,
          updatedAt: state.contests[0].createdAt,
        },
      ])
    if (resource === 'tags' && get)
      return list(
        [...new Set(state.problems.flatMap((p) => p.tags))].map((name, i) => ({
          id: i + 1,
          name,
          problemCount: state.problems.filter((p) => p.tags.includes(name)).length,
        })),
      )

    // Discussion endpoints share the same thread and ownership behavior across pages.
    if (action === 'discussions' && ['problems', 'editorials', 'contests'].includes(resource)) {
      const field =
        resource === 'problems'
          ? 'problemId'
          : resource === 'editorials'
            ? 'editorialId'
            : 'contestId'
      if (
        resource === 'editorials' &&
        editorialView(found(state.editorials.find((e) => e.id === id))).locked
      )
        throw new MockError(403, '通过题目后可见。')
      if (get) return list(state.discussions.filter((d) => d[field] === id))
      if (post) {
        const user = requireUser()
        const parentId = typeof body.parentId === 'number' ? body.parentId : undefined
        if (parentId) found(state.discussions.find((d) => d.id === parentId && d[field] === id))
        const discussion: DtoDiscussionResponse = {
          id: Math.max(0, ...state.discussions.map((d) => d.id)) + 1,
          [field]: id,
          parentId,
          contentMd: required('contentMd'),
          authorId: user.id,
          authorName: user.username,
          createdAt: isoNow(),
          updatedAt: isoNow(),
          edited: false,
        }
        state.discussions.push(discussion)
        return discussion
      }
    }
    if (resource === 'discussions' && id) {
      const discussion = found(state.discussions.find((d) => d.id === Number(id)))
      own(discussion.authorId)
      if (del) {
        const deleted = new Set([discussion.id])
        let added = true
        while (added) {
          added = false
          for (const post of state.discussions) {
            if (post.parentId && deleted.has(post.parentId) && !deleted.has(post.id)) {
              deleted.add(post.id)
              added = true
            }
          }
        }
        state.discussions = state.discussions.filter((post) => !deleted.has(post.id))
      } else if (method.toUpperCase() === 'PUT') {
        discussion.contentMd = required('contentMd')
        discussion.updatedAt = isoNow()
        discussion.edited = true
      } else throw new MockError(405, '不支持此操作。')
      return discussion
    }
    if (resource === 'problems' && get) {
      const items = state.problems.map((p) => ({ ...p, userStatus: progress(p.id) }))
      if (id) return found(items.find((p) => p.id === id))
      const keyword = String(params.keyword ?? '').toLowerCase()
      return list(
        items.filter(
          (p) =>
            `${p.title} ${p.source}`.toLowerCase().includes(keyword) &&
            (!params.tag || p.tags.includes(String(params.tag))) &&
            (!params.difficulty || p.difficulty === Number(params.difficulty)) &&
            (!params.status || p.userStatus === params.status),
        ),
      )
    }
    if (resource === 'submissions') {
      const user = requireUser()
      if (get && id) return found(state.submissions.find((s) => s.id === id))
      if (get)
        return list(
          state.submissions.filter(
            (s) =>
              (!params.user || [s.username, s.userId].includes(String(params.user))) &&
              (!params.problem || s.problemId === params.problem) &&
              (!params.contest || s.contestId === params.contest) &&
              (!params.language || s.language === params.language) &&
              (!params.status || s.status === params.status),
          ),
        )
      if (post && !id) {
        const problem = found(state.problems.find((p) => p.id === text('problemId')))
        if (!['cpp', 'c', 'python'].includes(text('language')))
          throw new MockError(400, '不支持此语言。')
        const contestId = text('contestId') || undefined
        if (contestId && !state.registrations.includes(contestId))
          throw new MockError(403, '请先报名比赛。')
        required('sourceCode')
        const submission: DtoSubmissionResponse = {
          id: crypto.randomUUID(),
          problemId: problem.id,
          problemTitle: problem.title,
          userId: user.id,
          username: user.username,
          language: text('language'),
          sourceCode: String(body.sourceCode),
          contestId,
          status: 'Pending',
          score: 0,
          judgedCases: 0,
          totalCases: 5,
          totalTimeMs: 0,
          peakMemoryKb: 0,
          submittedAt: isoNow(),
          caseResults: [],
        }
        state.submissions.unshift(submission)
        state.pending[submission.id] = { started: clock(), verdict: nextVerdict }
        problem.submissionCount++
        return submission
      }
    }
    if (resource === 'users' && get && id) {
      const user =
        id === 'demo'
          ? demoUser
          : id === 'lin'
            ? { ...demoUser, id: '00000002-0000-4000-8000-000000000001', username: 'lin' }
            : undefined
      const profileUser = found(user)
      const attempts = state.submissions.filter((s) => s.userId === profileUser.id)
      const solved = new Set(
        attempts.filter((s) => s.status === 'Accepted').map((s) => s.problemId),
      )
      return {
        userId: profileUser.id,
        username: profileUser.username,
        role: 'user',
        joinedAt: state.problems[0].createdAt,
        rating: 0,
        solvedCount: solved.size,
        attemptedCount: new Set(attempts.map((s) => s.problemId)).size,
        acceptedCount: attempts.filter((s) => s.status === 'Accepted').length,
        submissionCount: attempts.length,
        byDifficulty: Array.from({ length: 10 }, (_, i) => ({
          difficulty: i + 1,
          total: state.problems.filter((p) => p.difficulty === i + 1).length,
          solved: state.problems.filter((p) => p.difficulty === i + 1 && solved.has(p.id)).length,
        })),
        activity: Array.from({ length: 91 }, (_, i) => {
          const date = new Date(clock() - i * 86400000).toISOString().slice(0, 10)
          return { date, count: attempts.filter((s) => s.submittedAt.startsWith(date)).length }
        }),
      } satisfies DtoProfileResponse
    }
    if (resource === 'editorials') {
      if (get && !id)
        return list(
          state.editorials
            .map(editorialView)
            .filter(
              (e) =>
                ((e.status === 'published' && e.visibility === 'public') || e.canEdit) &&
                (!params.problem || e.problemId === params.problem) &&
                e.title.includes(String(params.keyword ?? '')),
            )
            .sort((a, b) =>
              params.sort === 'votes'
                ? b.voteCount - a.voteCount
                : b.createdAt.localeCompare(a.createdAt),
            ),
        )
      if (get) return editorialView(found(state.editorials.find((e) => e.id === id)))
      if (post && !id) {
        const user = requireUser()
        const problem = found(state.problems.find((p) => p.id === text('problemId')))
        const editorial: DtoEditorialResponse = {
          id: crypto.randomUUID(),
          title: required('title'),
          contentMd: required('contentMd'),
          problemId: problem.id,
          problemTitle: problem.title,
          authorId: user.id,
          authorName: user.username,
          canEdit: true,
          locked: false,
          createdAt: isoNow(),
          updatedAt: isoNow(),
          voteCount: 0,
          voted: false,
          solvedOnly: !!body.solvedOnly,
          status: body.status === 'draft' ? 'draft' : 'published',
          visibility: body.visibility === 'private' ? 'private' : 'public',
        }
        state.editorials.unshift(editorial)
        return editorial
      }
      const editorial = found(state.editorials.find((e) => e.id === id))
      if (post && action === 'vote') {
        requireUser()
        if (editorialView(editorial).locked) throw new MockError(403, '通过后才能赞同。')
        const up = body.up === true
        if (up !== editorial.voted) editorial.voteCount += up ? 1 : -1
        editorial.voted = up
        return { voted: up, voteCount: editorial.voteCount }
      }
      own(editorial.authorId)
      if (del) {
        state.editorials = state.editorials.filter((e) => e.id !== id)
        return { status: 'ok' }
      }
      if (method.toUpperCase() === 'PUT') {
        Object.assign(editorial, {
          title: required('title'),
          contentMd: required('contentMd'),
          updatedAt: isoNow(),
          solvedOnly: !!body.solvedOnly,
          status: body.status === 'draft' ? 'draft' : 'published',
          visibility: body.visibility === 'private' ? 'private' : 'public',
        })
        return editorialView(editorial)
      }
    }
    if (resource === 'problem-sets') {
      if (get && !id)
        return list(
          state.sets
            .filter(
              (s) =>
                (s.visibility === 'public' || s.authorId === state.user?.id) &&
                s.title.includes(String(params.keyword ?? '')),
            )
            .map(setView),
        )
      if (get) return setView(found(state.sets.find((s) => s.id === id)))
      if (post && !id) {
        const user = requireUser()
        const set: DtoSetResponse = {
          id: crypto.randomUUID(),
          title: required('title'),
          description: text('description'),
          authorId: user.id,
          authorName: user.username,
          visibility: body.visibility === 'private' ? 'private' : 'public',
          createdAt: isoNow(),
          updatedAt: isoNow(),
          items: [],
          canEdit: true,
          problemCount: 0,
          solvedCount: 0,
        }
        state.sets.unshift(set)
        return set
      }
      const set = found(state.sets.find((s) => s.id === id))
      own(set.authorId)
      if (del) {
        state.sets = state.sets.filter((s) => s.id !== id)
        return { status: 'ok' }
      }
      if (method.toUpperCase() === 'PUT') {
        if (action === 'items' && Array.isArray(body.items)) {
          set.items = (body.items as DtoSetItemRequest[]).map((item, i) => {
            const p = found(state.problems.find((p) => p.id === item.problemId))
            return {
              problemId: p.id,
              title: p.title,
              difficulty: p.difficulty,
              tags: p.tags,
              visibility: p.visibility,
              userStatus: progress(p.id),
              note: item.note || '',
              sortOrder: i,
              acceptCount: p.acceptedCount,
              submitCount: p.submissionCount,
            }
          })
        } else
          Object.assign(set, {
            title: required('title'),
            description: text('description'),
            visibility: body.visibility === 'private' ? 'private' : 'public',
          })
        set.updatedAt = isoNow()
        return setView(set)
      }
    }
    if (resource === 'contests') {
      if (get && !id) return list(state.contests)
      const contest = found(state.contests.find((c) => c.id === id))
      const problems: DtoContestProblemResponse[] = state.problems.slice(0, 6).map((p, i) => ({
        contestId: id,
        problemId: p.id,
        title: p.title,
        difficulty: p.difficulty,
        tags: p.tags,
        visibility: 'public',
        label: String.fromCharCode(65 + i),
        sortOrder: i,
        points: 100,
        color: '',
      }))
      if (get && !action) return { contest, problems, staffRole: '' }
      if (get && action === 'registration') {
        requireUser()
        return { registered: state.registrations.includes(id) }
      }
      if (post && action === 'register') {
        requireUser()
        if (!state.registrations.includes(id)) state.registrations.push(id)
        return { status: 'ok' }
      }
      if (get && action === 'problems')
        return {
          ...found(state.problems.find((p) => p.id === childId)),
          ...found(problems.find((p) => p.problemId === childId)),
        }
      if (get && action === 'rankboard') {
        return {
          format: 'icpc',
          frozen: false,
          juryView: false,
          problemCount: problems.length,
          problemIds: problems.map((p) => p.problemId),
          problems,
          rows: ['lin', 'demo', 'mori', 'sora'].map((username, row) => ({
            username,
            userId: username === 'demo' ? demoUser.id : username,
            rank: row + 1,
            solved: 5 - row,
            score: (5 - row) * 100,
            penalty: 1800 + row * 1200,
            hasPending: false,
            cells: problems.map((_, c) => ({
              attempts: c < 5 - row ? 1 + (c % 2) : 0,
              score: c < 5 - row ? 100 : 0,
              penaltySec: 1200 + c * 600,
              solvedAt:
                c < 5 - row
                  ? new Date(Date.parse(contest.beginAt) + 1200000).toISOString()
                  : undefined,
              firstSolver: row === 0 && c === 0,
              pendingCount: 0,
            })),
          })),
        } satisfies DtoRankboardResponse
      }
      if (action === 'clarifications') {
        requireUser()
        if (get) return list(state.clarifications[id] ?? [])
        if (post) {
          const item = {
            id: clock(),
            subject: required('subject'),
            body: required('body'),
            authorName: requireUser().username,
            createdAt: isoNow(),
            fromJury: false,
            announce: false,
            answered: false,
            problemId: text('problemId') || undefined,
            replies: [],
          }
          ;(state.clarifications[id] ??= []).push(item)
          return item
        }
      }
    }
    throw new MockError(501, '此接口尚未提供 mock，未向真实后端发送请求。')
  }
  return {
    state,
    get scenario() {
      return scenario
    },
    set scenario(value: MockScenario) {
      scenario = value
    },
    get nextVerdict() {
      return nextVerdict
    },
    set nextVerdict(value: string) {
      nextVerdict = value
    },
    handle(request: MockRequest) {
      return structuredClone(route(request))
    },
  }
}
