import { describe, expect, it } from 'vitest'
import type {
  DtoDiscussionResponse,
  DtoEditorialResponse,
  DtoProblemResponse,
  DtoSubmissionResponse,
} from '@/generated/api/model'
import { createMockAPI, MockError } from './api'
import { createFixtures } from './fixtures'

const now = Date.UTC(2026, 8, 6, 12)
const make = () => createMockAPI(createFixtures(now), () => now)

describe('stateful mock API', () => {
  it('allows a resource owner into their workspace without granting site administration', () => {
    const api = make()
    const owned = api.state.problems[0]
    owned.ownerId = api.state.user!.id
    expect(
      api.handle({ method: 'GET', path: `/api/admin/problems/${owned.id}/package` }),
    ).toHaveProperty('meta.problemId', owned.id)
    expect(() => api.handle({ method: 'GET', path: '/api/admin/users' })).toThrow('管理员权限')
    expect(() =>
      api.handle({ method: 'DELETE', path: `/api/admin/problems/${api.state.problems[1].id}` }),
    ).toThrow('协作权限')
  })
  it('recomputes problem capabilities when switching independent identities', () => {
    const api = make()
    const read = () =>
      api.handle({
        method: 'GET',
        path: `/api/problems/${api.state.problems[0].id}`,
      }) as DtoProblemResponse
    expect(read().permissions.readPackage).toBe(false)
    api.handle({
      method: 'POST',
      path: '/api/auth/login',
      body: { username: 'admin_demo', password: 'demo123' },
    })
    expect(read().permissions.manageAccess).toBe(true)
    api.handle({ method: 'POST', path: '/api/auth/logout' })
    expect(read().permissions.view).toBe(true)
    expect(read().permissions.readPackage).toBe(false)
  })
  it('filters before pagination and returns the filtered total', () => {
    const api = make()
    const result = api.handle({
      method: 'GET',
      path: '/api/problems',
      params: { tag: '图论', difficulty: 4, size: 1, page: 2 },
    }) as { items: DtoProblemResponse[]; total: number }
    expect(result.total).toBe(2)
    expect(result.items.map((p) => p.title)).toEqual(['连通分量'])
    expect(
      api.handle({ method: 'GET', path: '/api/problems', params: { keyword: '不存在的标题' } }),
    ).toEqual({ items: [], total: 0 })
  })

  it('simulates elapsed judging, keeps source intact, and updates progress exactly once', () => {
    let clock = now
    const api = createMockAPI(createFixtures(now), () => clock)
    const problem = api.state.problems[0]
    const initialCount = problem.submissionCount
    const initialAccepted = problem.acceptedCount
    const sourceCode = '// this is not executed\nint main() {}\n'
    const created = api.handle({
      method: 'POST',
      path: '/api/submissions',
      body: { problemId: problem.id, language: 'cpp', sourceCode },
    }) as DtoSubmissionResponse
    expect(created.status).toBe('Pending')
    expect(problem.submissionCount).toBe(initialCount)
    clock += 2400
    const progress = api.handle({
      method: 'GET',
      path: `/api/submissions/${created.id}/progress`,
    }) as DtoSubmissionResponse
    expect(progress.status).toBe('Judging')
    expect(progress.judgedCases).toBeGreaterThan(0)
    clock += 3000
    const done = api.handle({
      method: 'GET',
      path: `/api/submissions/${created.id}`,
    }) as DtoSubmissionResponse
    expect(done.status).toBe('Accepted')
    expect(done.sourceCode).toBe(sourceCode)
    expect(done.totalCases).toBe(80)
    expect(done.judgedCases).toBe(done.totalCases)
    expect(done.caseResults).toHaveLength(done.totalCases)
    expect(created.status).toBe('Pending') // Prior responses are snapshots, not mutable aliases.
    api.handle({ method: 'GET', path: `/api/submissions/${created.id}` })
    expect(problem.acceptedCount).toBe(initialAccepted + 1)
    expect(problem.submissionCount).toBe(initialCount + 1)
  })

  it('captures the verdict at submit time, including compile errors', () => {
    let clock = now
    const api = createMockAPI(createFixtures(now), () => clock)
    api.nextVerdict = 'Compile Error'
    const result = api.handle({
      method: 'POST',
      path: '/api/submissions',
      body: { problemId: api.state.problems[0].id, language: 'cpp', sourceCode: 'invalid' },
    }) as DtoSubmissionResponse
    api.nextVerdict = 'Accepted'
    clock += 5000
    const done = api.handle({
      method: 'GET',
      path: `/api/submissions/${result.id}`,
    }) as DtoSubmissionResponse
    expect(done.status).toBe('Compile Error')
    expect(done.compileResult).toContain('模拟编译错误')
    expect(done.caseResults).toEqual([])
  })

  it('persists threaded discussion mutations and rejects cross-thread replies', () => {
    const api = make()
    const path = `/api/problems/${api.state.problems[0].id}/discussions`
    const created = api.handle({
      method: 'POST',
      path,
      body: { contentMd: '边界情况怎么处理？', parentId: 1 },
    }) as DtoDiscussionResponse
    api.handle({
      method: 'PUT',
      path: `/api/discussions/${created.id}`,
      body: { contentMd: '已理解，谢谢！' },
    })
    const read = api.handle({ method: 'GET', path }) as { items: DtoDiscussionResponse[] }
    expect(read.items.find((d) => d.id === created.id)).toMatchObject({
      parentId: 1,
      contentMd: '已理解，谢谢！',
      edited: true,
    })
    expect(() =>
      api.handle({ method: 'POST', path, body: { contentMd: 'reply', parentId: 3 } }),
    ).toThrow(MockError)
    expect(() => api.handle({ method: 'DELETE', path: '/api/discussions/1' })).toThrow(
      '没有此内容的操作权限',
    )
    const reply = api.handle({
      method: 'POST',
      path,
      body: { contentMd: 'nested', parentId: created.id },
    }) as DtoDiscussionResponse
    api.handle({ method: 'DELETE', path: `/api/discussions/${created.id}` })
    expect(api.state.discussions.some((d) => d.id === reply.id || d.id === created.id)).toBe(false)
  })

  it('makes voting idempotent and respects spoiler settings', () => {
    const api = make()
    const editorial = api.state.editorials[0]
    const count = editorial.voteCount
    const path = `/api/editorials/${editorial.id}/vote`
    api.handle({ method: 'POST', path, body: { up: true } })
    api.handle({ method: 'POST', path, body: { up: true } })
    expect(editorial.voteCount).toBe(count + 1)
    api.handle({ method: 'POST', path, body: { up: false } })
    expect(editorial.voteCount).toBe(count)
    editorial.solvedOnly = true
    api.state.submissions = []
    const locked = api.handle({
      method: 'GET',
      path: `/api/editorials/${editorial.id}`,
    }) as DtoEditorialResponse
    expect(locked.locked).toBe(true)
    expect(locked.contentMd).toBe('')
  })

  it('supports empty and failure states without preventing session recovery', () => {
    const api = make()
    api.scenario = 'empty'
    expect(api.handle({ method: 'GET', path: '/api/problems' })).toEqual({ items: [], total: 0 })
    api.scenario = 'error'
    expect(() => api.handle({ method: 'GET', path: '/api/problems' })).toThrow('模拟加载失败')
    expect(api.handle({ method: 'POST', path: '/api/auth/refresh' })).toHaveProperty(
      'user.username',
      'demo',
    )
    api.handle({ method: 'POST', path: '/api/auth/logout' })
    expect(() => api.handle({ method: 'POST', path: '/api/auth/refresh' })).toThrow('请先登录')
  })

  it('fails explicitly for unsupported routes', () => {
    const api = make()
    api.state.user!.role = 'admin'
    expect(() => api.handle({ method: 'GET', path: '/api/admin/unsupported' })).toThrow(
      '未向真实后端发送请求',
    )
    expect(() => api.handle({ method: 'GET', path: '/internal/judge/v1/jobs/claim' })).toThrow(
      MockError,
    )
  })
})
