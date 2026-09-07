import type {
  DtoAuthResponse,
  DtoContestProblemResponse,
  DtoContestResponse,
  DtoProfileResponse,
  DtoRankboardResponse,
  DtoSubmissionResponse,
} from '@/generated/api/model'
import { createFixtures, type MockState } from './fixtures'
import { authoringRequest } from './authoring'
import { adminReadRequest } from './console'
import { adminUser, mockUsers, contestantUser, juryUser, observerUser } from './identities'
import { officialDomainID, problemPermissions } from './problem-permissions'
import { contestPermissions } from './contest-permissions'
import { rejudgingRequest } from './rejudging'
import { problemSetRequest } from './problem-sets'
import { contentRequest } from './content'
import { MockError } from './errors'
import { allocateReference, initializeReferences, resolveMockRequest } from './references'
import { initializeDomains, domainView, scopeFor, createDomainSpace } from './domains'
import { mockCan, mockManager } from './domain-policy'
export { MockError } from './errors'

export type MockRequest = {
  method: string
  path: string
  params?: Record<string, unknown>
  body?: Record<string, unknown>
}
export type MockScenario = 'normal' | 'slow' | 'empty' | 'error'
export const demoPassword = 'demo123'

/** A small stateful API for UI development, not a judge or an authorization simulator. */
function createResourceAPI(state: MockState, clock: () => number) {
  const domainID = state.scope?.id ?? officialDomainID
  state.submissionGenerations ??= {}
  state.rejudgeBatches ??= []
  state.problemDrafts ??= {}
  state.problemCandidateSamples ??= {}
  state.problemReleases ??= {}
  state.buildInputs ??= {}
  state.contestProblemVersions ??= {}
  state.setGrants ??= {}
  state.nextSetGrantId ??= 0
  state.editorialVotes ??= {}
  state.nextDiscussionId = Math.max(
    state.nextDiscussionId ?? 0,
    ...state.discussions.map((post) => post.id),
  )
  for (const editorial of state.editorials) editorial.domainId ??= domainID
  for (const post of state.discussions) post.domainId ??= domainID
  for (const set of state.sets) {
    set.ownerId ??= set.authorId ?? adminUser.id
    set.ownerName ??= set.authorName
    set.domainId ??= domainID
  }
  for (const problem of state.problems) {
    problem.publishedVersion ??= 1
    if (problem.publishedVersion && !state.problemReleases[problem.id])
      state.problemReleases[problem.id] = [
        {
          problem: structuredClone(problem),
          release: {
            version: problem.publishedVersion,
            revision: 1,
            artifactVersion: 1,
            language: 'zh',
            sha256: 'mock-initial',
            caseCount: 1,
            createdAt: problem.createdAt,
          },
        },
      ]
    problem.ownerId ??= problem.authorId ?? adminUser.id
    problem.domainId ??= domainID
    problem.permissions = problemPermissions(problem, state.user, state.scope)
  }
  for (const contest of state.contests) {
    contest.ownerId ??= contest.createdBy ?? adminUser.id
    contest.domainId ??= domainID
    contest.admission ??= 'members'
  }
  initializeReferences(state)
  state.clarificationRecipients ??= {}
  state.staff ??= {
    [state.contests[0].id]: [juryUser, observerUser].map((user) => ({
      userId: user.id,
      username: user.username,
      role: user.id === juryUser.id ? 'jury' : 'observer',
      createdAt: state.contests[0].createdAt,
    })),
  }
  state.registrations[contestantUser.id] ??= state.contests[0] ? [state.contests[0].id] : []
  for (const [id, problems] of Object.entries(state.contestProblemIds)) {
    state.contestProblemVersions[id] ??= {}
    for (const problemId of problems)
      state.contestProblemVersions[id][problemId] ??=
        state.problems.find((p) => p.id === problemId)?.publishedVersion ?? 1
  }
  for (const sub of state.submissions) sub.problemVersion ??= 1
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
  const releaseProblem = (problemId: string, version: number) =>
    found(
      state.problemReleases[problemId]?.find((entry) => entry.release.version === version)?.problem,
    )
  const staffRole = (contestId: string) =>
    state.staff[contestId]?.find((s) => s.userId === state.user?.id)?.role ?? ''
  const contestCaps = (contestId: string) =>
    contestPermissions(
      found(state.contests.find((c) => c.id === contestId)),
      state.user,
      staffRole(contestId),
      registered(contestId),
      [],
      state.scope,
    )
  const isStaff = (contestId: string) => contestCaps(contestId).viewJury
  const canReply = (contestId: string) => contestCaps(contestId).reply
  const registered = (contestId: string) =>
    (state.registrations[state.user?.id ?? ''] ?? []).includes(contestId)
  const problemVisible = (problemId: string) =>
    state.problems.some(
      (p) => p.id === problemId && problemPermissions(p, state.user, state.scope).view,
    )
  function submissionVisible(submission: DtoSubmissionResponse) {
    if (submission.userId === state.user?.id || mockManager(state.scope, state.user)) return true
    if (!submission.contestId) return problemVisible(submission.problemId)
    if (isStaff(submission.contestId)) return true
    const contest = state.contests.find((c) => c.id === submission.contestId)
    return (
      !!contest &&
      Date.parse(contest.endAt) < clock() &&
      contest.rankboardVisible &&
      problemVisible(submission.problemId)
    )
  }
  function progress(problemId: string): 'solved' | 'attempted' | 'none' {
    const attempts = state.submissions.filter(
      (s) => s.problemId === problemId && s.userId === state.user?.id && !s.contestId,
    )
    return attempts.some((s) => s.status === 'Accepted')
      ? 'solved'
      : attempts.length
        ? 'attempted'
        : 'none'
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
        if (problem && !submission.contestId && job.verdict === 'Accepted') problem.acceptedCount++
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
    if (parts[0] !== 'api') throw new MockError(501, '此接口尚未提供 mock，未向真实后端发送请求。')
    if (resource === 'admin') {
      const actor = requireUser()
      if (scenario === 'error' && get)
        throw new MockError(503, '模拟加载失败，请切回正常场景后重试。')
      if (id === 'contests') {
        if (get && !action)
          return list(
            state.contests
              .filter((contest) => contestCaps(contest.id).edit)
              .map((contest) => ({ ...contest, permissions: contestCaps(contest.id) })),
          )
        const existing = action
          ? found(state.contests.find((contest) => contest.id === action))
          : undefined
        if (existing && !contestCaps(existing.id).edit)
          throw new MockError(403, '没有编辑比赛的权限')
        if (get && existing) return route({ method: 'GET', path: `/api/contests/${existing.id}` })
        if (method === 'PUT' && existing && childId === 'problems') {
          if (!Array.isArray(body.problems) || body.problems.length > 100)
            throw new MockError(400, '题目编排无效')
          const entries = body.problems.map((entry) => {
            const value = entry as Record<string, unknown>,
              problem = found(state.problems.find((p) => p.id === value.problemId))
            const previous = state.contestProblemVersions[existing.id]?.[problem.id]
            if (!previous && (!problemVisible(problem.id) || !problem.publishedVersion))
              throw new MockError(400, '题目不可用或尚未发布')
            const label = String(value.label ?? ''),
              points = Number(value.points ?? 100)
            if (!/^[A-Za-z0-9]{1,8}$/.test(label) || !Number.isInteger(points) || points < 0)
              throw new MockError(400, '题目编号或分值无效')
            return { problemId: problem.id, label, color: String(value.color ?? ''), points }
          })
          if (
            new Set(entries.map((entry) => entry.label)).size !== entries.length ||
            new Set(entries.map((entry) => entry.problemId)).size !== entries.length
          )
            throw new MockError(400, '题目或编号重复')
          state.contestProblemIds[existing.id] = entries.map((entry) => entry.problemId)
          const versions = (state.contestProblemVersions[existing.id] ??= {})
          for (const entry of entries)
            versions[entry.problemId] ||= found(
              state.problems.find((p) => p.id === entry.problemId),
            ).publishedVersion
          state.contestProblemVersions[existing.id] = Object.fromEntries(
            entries.map((entry) => [entry.problemId, versions[entry.problemId]]),
          )
          ;(state.contestEntries ??= {})[existing.id] = entries
          return { status: 'updated' }
        }
        if ((post && !action) || (method === 'PUT' && existing && !childId)) {
          if (!existing && !mockCan(state.scope, actor, 'contest.create'))
            throw new MockError(403, '当前域没有创建比赛权限')
          const title = required('title'),
            beginAt = required('beginAt'),
            endAt = required('endAt')
          if (
            !Number.isFinite(Date.parse(beginAt)) ||
            Date.parse(endAt) <= Date.parse(beginAt) ||
            !Number.isFinite(Date.parse(endAt))
          )
            throw new MockError(400, '比赛时间无效')
          const rule = text('rule') || existing?.rule || 'icpc',
            feedback = text('feedback') || existing?.feedback || 'full',
            admission = text('admission') || existing?.admission || 'members',
            visibility = text('visibility') || existing?.visibility || 'public'
          if (
            !['acm', 'icpc', 'ioi', 'oi'].includes(rule) ||
            !['full', 'none', 'summary'].includes(feedback) ||
            !['members', 'restricted'].includes(admission) ||
            !['public', 'private', 'password'].includes(visibility)
          )
            throw new MockError(400, '比赛设置无效')
          if (
            visibility === 'password' &&
            !text('password') &&
            (!existing || existing.visibility !== 'password')
          )
            throw new MockError(400, '请设置比赛密码')
          const item: DtoContestResponse = {
            id: existing?.id ?? crypto.randomUUID(),
            publicId: existing?.publicId ?? allocateReference(state, 'contests'),
            domainId: domainID,
            ownerId: existing?.ownerId ?? actor.id,
            createdBy: existing?.createdBy ?? actor.id,
            createdAt: existing?.createdAt ?? isoNow(),
            title,
            beginAt,
            endAt,
            description: text('description'),
            visibility,
            rule: rule as DtoContestResponse['rule'],
            format: (rule === 'acm' ? 'icpc' : rule) as DtoContestResponse['format'],
            feedback: feedback as DtoContestResponse['feedback'],
            admission: admission as DtoContestResponse['admission'],
            freezeAt: text('freezeAt') || undefined,
            unfreezeAt: text('unfreezeAt') || undefined,
            rankboardVisible: body.rankboardVisible !== false,
            penalizeCompileError: body.penalizeCompileError === true,
            penaltyMinutes: Number(body.penaltyMinutes ?? 20),
            permissions: {} as DtoContestResponse['permissions'],
          }
          if (existing) Object.assign(existing, item)
          else {
            state.contests.unshift(item)
            state.contestProblemIds[item.id] = []
            state.contestProblemVersions[item.id] = {}
          }
          if (text('password')) (state.contestPasswords ??= {})[item.id] = text('password')
          item.permissions = contestCaps(item.id)
          return item
        }
        throw new MockError(501, '此比赛管理操作尚未提供 mock')
      }
      if (id === 'rejudgings')
        return rejudgingRequest(
          state,
          { method, path, params, body },
          clock(),
          contestCaps,
          nextVerdict,
        )
      if (id !== 'problems' && id !== 'package-templates' && actor.role !== 'admin')
        throw new MockError(403, '此操作需要站点管理员权限。')
      if (get && id !== 'problems' && id !== 'package-templates')
        return adminReadRequest(state, { method, path, params, body }, clock())
      return authoringRequest(state, { method, path, params, body }, clock())
    }
    const adoptingVersion =
      method === 'PUT' &&
      resource === 'contests' &&
      action === 'problems' &&
      parts.length === 6 &&
      parts[5] === 'version'
    if (parts.length > 5 && !adoptingVersion)
      throw new MockError(501, '此接口尚未提供 mock，未向真实后端发送请求。')
    if (resource === 'auth') {
      if (post && id === 'login') {
        const account = mockUsers.find((user) => user.username === text('username'))
        if (!account || text('password') !== demoPassword)
          throw new MockError(401, '请使用演示账号，密码均为 demo123。')
        state.user = { ...account }
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

    if (
      resource === 'editorials' ||
      resource === 'discussions' ||
      (action === 'discussions' && ['problems', 'contests'].includes(resource))
    )
      return contentRequest(state, { method, path, params, body }, clock(), progress)

    if (resource === 'problems' && get) {
      const items = state.problems
        .filter((p) =>
          id ? problemVisible(p.id) : p.visibility === 'public' && p.publishedVersion > 0,
        )
        .map((p) => ({
          ...p,
          permissions: problemPermissions(p, state.user, state.scope),
          userStatus: progress(p.id),
        }))
      if (id) {
        const item = found(items.find((p) => p.id === id))
        if (!item.publishedVersion && item.permissions.readPackage)
          return authoringRequest(
            state,
            { method: 'GET', path: `/api/admin/problems/${id}` },
            clock(),
          )
        return item
      }
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
      if (get && id) {
        const item = found(state.submissions.find((s) => s.id === id && submissionVisible(s)))
        return {
          ...item,
          sourceCode:
            item.userId === user.id ||
            mockManager(state.scope, user) ||
            (item.contestId
              ? isStaff(item.contestId)
              : state.problems.some((p) => p.id === item.problemId && p.ownerId === user.id))
              ? item.sourceCode
              : undefined,
        }
      }
      if (get)
        return list(
          state.submissions.filter(
            (s) =>
              submissionVisible(s) &&
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
        if (contestId && !contestCaps(contestId).submit)
          throw new MockError(403, '需要有效参赛资格和报名；观察员不能提交。')
        if (!contestId && !problemVisible(problem.id)) throw new MockError(404, '题目不存在。')
        if (contestId && !state.contestProblemIds[contestId]?.includes(problem.id))
          throw new MockError(404, '比赛题目不存在。')
        const version = contestId
          ? state.contestProblemVersions[contestId][problem.id]
          : problem.publishedVersion
        if (!version) throw new MockError(409, '题目尚未发布可评测版本。')
        required('sourceCode')
        const submission: DtoSubmissionResponse = {
          problemVersion: version,
          publicId: allocateReference(state, 'submissions'),
          problemPublicId: problem.publicId,
          contestPublicId: state.contests.find((c) => c.id === contestId)?.publicId,
          id: crypto.randomUUID(),
          problemId: problem.id,
          problemTitle: releaseProblem(problem.id, version).title,
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
      const user = mockUsers.find((user) => user.username === id)
      const profileUser = found(user)
      const attempts = state.submissions.filter(
        (s) => s.userId === profileUser.id && !s.contestId && problemVisible(s.problemId),
      )
      const solved = new Set(
        attempts.filter((s) => s.status === 'Accepted').map((s) => s.problemId),
      )
      return {
        userId: profileUser.id,
        username: profileUser.username,
        role: profileUser.role,
        joinedAt: state.problems[0]?.createdAt ?? '2026-09-01T00:00:00Z',
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
    if (resource === 'problem-sets')
      return problemSetRequest(
        state,
        { method, path, params, body },
        clock(),
        problemVisible,
        progress,
      )
    if (resource === 'contests') {
      if (get && !id)
        return list(
          state.contests
            .filter((c) => contestCaps(c.id).view)
            .map((c) => ({ ...c, permissions: contestCaps(c.id) })),
        )
      const contest = found(state.contests.find((c) => c.id === id))
      const permissions = contestCaps(id)
      if (!permissions.view) throw new MockError(404, '比赛不存在。')
      const problems: DtoContestProblemResponse[] = state.contestProblemIds[contest.id].map(
        (problemId, i) => {
          const version = state.contestProblemVersions[contest.id][problemId]
          const p = releaseProblem(problemId, version)
          return {
            version,
            problemPublicId: p.publicId,
            contestPublicId: contest.publicId,
            contestId: id,
            problemId: p.id,
            title: p.title,
            difficulty: p.difficulty,
            tags: p.tags,
            visibility: 'public',
            label: state.contestEntries?.[id]?.[i]?.label ?? String.fromCharCode(65 + i),
            sortOrder: i,
            points: state.contestEntries?.[id]?.[i]?.points ?? 100,
            color: state.contestEntries?.[id]?.[i]?.color ?? '',
          }
        },
      )
      if (get && !action)
        return { contest: { ...contest, permissions }, problems, staffRole: staffRole(id) }
      if (get && action === 'registration') {
        requireUser()
        return { registered: registered(id) }
      }
      if (post && action === 'register') {
        const user = requireUser()
        if (!permissions.register) throw new MockError(403, '当前身份不能报名。')
        if (Date.parse(contest.beginAt) <= clock()) throw new MockError(400, '报名已经结束。')
        if (contest.visibility === 'password' && text('password') !== state.contestPasswords?.[id])
          throw new MockError(403, '比赛密码不正确')
        if (!registered(id)) (state.registrations[user.id] ??= []).push(id)
        return { status: 'ok' }
      }
      if (get && action === 'problems') {
        requireUser()
        if (
          !permissions.previewProblems &&
          (!registered(id) || Date.parse(contest.beginAt) > clock())
        )
          throw new MockError(403, '报名且比赛开始后才能查看题目。')
        return {
          ...releaseProblem(childId, state.contestProblemVersions[id][childId]),
          ...found(problems.find((p) => p.problemId === childId)),
        }
      }
      if (adoptingVersion) {
        requireUser()
        if (!permissions.rejudge || !problemVisible(childId))
          throw new MockError(403, '没有切换版本权限。')
        const current = state.contestProblemVersions[id]?.[childId]
        if (!current) throw new MockError(404, '比赛题目不存在。')
        if (current === Number(body.version)) return { status: 'updated' }
        if (current !== Number(body.expectedVersion))
          throw new MockError(409, '比赛版本已变化，请刷新。')
        releaseProblem(childId, Number(body.version))
        state.contestProblemVersions[id][childId] = Number(body.version)
        return { status: 'updated' }
      }
      if (get && action === 'rankboard') {
        if (params.view === 'jury' && !isStaff(id)) throw new MockError(403, '没有赛务权限。')
        return {
          format: 'icpc',
          frozen: false,
          juryView: params.view === 'jury' && isStaff(id),
          problemCount: problems.length,
          problemIds: problems.map((p) => p.problemId),
          problems,
          rows: ['lin', 'contestant', 'demo', 'sora'].map((username, row) => ({
            username,
            userId: mockUsers.find((user) => user.username === username)?.id ?? username,
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
      if (action === 'staff') {
        requireUser()
        if (!isStaff(id)) throw new MockError(403, '没有赛务权限。')
        if (get) return list(state.staff[id] ?? [])
        if (!permissions.manageAccess)
          throw new MockError(403, '只有 owner 或域资源管理者可以修改赛务授权。')
        if (del) {
          state.staff[id] = (state.staff[id] ?? []).filter((s) => s.userId !== childId)
          return { status: 'ok' }
        }
        const user = found(mockUsers.find((user) => user.username === text('username')))
        const staff = {
          userId: user.id,
          username: user.username,
          role: body.role === 'observer' ? ('observer' as const) : ('jury' as const),
          createdAt: isoNow(),
        }
        state.staff[id] = [...(state.staff[id] ?? []).filter((s) => s.userId !== user.id), staff]
        return staff
      }
      if (action === 'clarifications') {
        const user = requireUser()
        const nextId = () =>
          1 +
          Math.max(
            0,
            ...Object.values(state.clarifications).flatMap((items) =>
              items.flatMap((item) => [item.id, ...item.replies.map((reply) => reply.id)]),
            ),
          )
        if (!isStaff(id) && !registered(id) && contest.visibility !== 'public')
          throw new MockError(403, '报名后才能查看澄清。')
        if (get)
          return list(
            (state.clarifications[id] ?? []).filter(
              (item) =>
                isStaff(id) ||
                item.announce ||
                item.authorName === user.username ||
                state.clarificationRecipients[item.id] === user.id,
            ),
          )
        if (post) {
          if (childId === 'reply') {
            if (!canReply(id)) throw new MockError(403, '只有裁判可以回复澄清。')
            const parent = body.parentId
              ? found(state.clarifications[id]?.find((item) => item.id === body.parentId))
              : undefined
            const recipient = text('recipientId')
            if (
              recipient &&
              !(state.registrations[recipient] ?? []).includes(id) &&
              !state.staff[id]?.some((member) => member.userId === recipient)
            )
              throw new MockError(400, '接收者必须是本场参赛者或赛务成员。')
            const reply = {
              id: nextId(),
              subject: parent?.subject ?? text('subject'),
              body: required('body'),
              authorName: user.username,
              createdAt: isoNow(),
              fromJury: true,
              announce: !parent && !recipient,
              answered: true,
              parentId: parent?.id,
              replies: [],
            }
            if (recipient) state.clarificationRecipients[reply.id] = recipient
            if (parent) {
              parent.replies.push(reply)
              parent.answered = true
            } else (state.clarifications[id] ??= []).push(reply)
            return reply
          }
          const item = {
            id: nextId(),
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
      return structuredClone(route(resolveMockRequest(state, request)))
    },
  }
}

/** Each domain owns a separate resource graph; account identity stays shared. */
export function createMockAPI(state: MockState = createFixtures(), clock = Date.now) {
  initializeDomains(state)
  const official = state.domains!.find((domain) => domain.slug === 'official')!
  state.scope = scopeFor(official, state.user)
  const root = createResourceAPI(state, clock)
  const spaces = new Map<string, ReturnType<typeof createResourceAPI>>()
  return {
    state,
    get scenario() {
      return root.scenario
    },
    set scenario(value: MockScenario) {
      root.scenario = value
    },
    get nextVerdict() {
      return root.nextVerdict
    },
    set nextVerdict(value: string) {
      root.nextVerdict = value
    },
    handle(request: MockRequest): unknown {
      request = { ...request, method: request.method.toUpperCase() }
      const parts = request.path.split('/').filter(Boolean)
      if (parts[1] === 'domains' && parts.length <= 3) {
        if (request.method !== 'GET')
          throw new MockError(501, '此域治理接口尚未提供 mock，未向真实后端发送请求。')
        const views = state.domains!.map((domain) => domainView(domain, state.user))
        if (parts.length === 3) {
          const value = views.find((domain) => domain.slug === decodeURIComponent(parts[2]))
          if (!value || (!value.canEnter && !['pending', 'invited'].includes(value.memberStatus)))
            throw new MockError(404, '域不存在或不可访问')
          return structuredClone(value)
        }
        const keyword = String(request.params?.keyword ?? '').toLowerCase()
        const items = views.filter(
          (domain) =>
            (domain.canEnter || ['pending', 'invited'].includes(domain.memberStatus)) &&
            `${domain.name} ${domain.slug}`.toLowerCase().includes(keyword),
        )
        const page = Math.max(1, Number(request.params?.page) || 1),
          size = Math.min(100, Math.max(1, Number(request.params?.size) || 50))
        return structuredClone({
          items: items.slice((page - 1) * size, page * size),
          total: items.length,
        })
      }
      const scoped = parts[1] === 'domains'
      const slug = scoped ? decodeURIComponent(parts[2]) : 'official'
      const domain = state.domains!.find((domain) => domain.slug === slug)
      if (!domain) throw new MockError(404, '域不存在或不可访问')
      const path = scoped ? '/api/' + parts.slice(3).join('/') : request.path
      const global = !scoped && /^\/api\/(auth\/|admin\/(?:stats|users)(?:\/|$))/.test(path)
      if (!global && !domainView(domain, state.user).canEnter)
        throw new MockError(404, '域不存在或不可访问')
      if (
        scoped &&
        /^\/api\/(auth(?:\/|$)|admin\/(?:stats|users|tags|announcements)(?:\/|$))/.test(path)
      )
        throw new MockError(404, '此接口不属于域资源')
      if (!global && domain.archived && request.method !== 'GET')
        throw new MockError(403, '域已归档，只能读取')
      let api = root
      let data = state
      if (slug !== 'official') {
        data = state.domainSpaces![slug] ??= createDomainSpace(domain, clock())
        data.user = state.user
        data.scope = scopeFor(domain, state.user)
        api = spaces.get(slug) ?? createResourceAPI(data, clock)
        spaces.set(slug, api)
      } else state.scope = scopeFor(domain, state.user)
      api.scenario = root.scenario
      api.nextVerdict = root.nextVerdict
      if (
        path === '/api/submissions' &&
        request.method === 'POST' &&
        state.user &&
        !mockCan(data.scope, state.user, 'submission.create')
      )
        throw new MockError(403, '当前域没有提交权限')
      return api.handle({ ...request, path })
    },
  }
}
