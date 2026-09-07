import { describe, expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, demoUser, juryUser } from './identities'
import type { DtoDiscussionResponse, DtoEditorialResponse } from '@/generated/api/model'

describe('content parent permissions in mock mode', () => {
  const setup = () => {
    let now = Date.UTC(2026, 8, 7, 12)
    const api = createMockAPI(createFixtures(now), () => now)
    return {
      api,
      advance: () => {
        now += 6000
      },
    }
  }
  it('permits moderation without rewriting someone else’s editorial or comment', () => {
    const { api } = setup(),
      e = api.state.editorials[0]
    api.state.user = { ...adminUser }
    const visible = api.handle({
      method: 'GET',
      path: `/api/editorials/${e.id}`,
    }) as DtoEditorialResponse
    expect(visible.permissions).toMatchObject({ edit: false, delete: true })
    expect(() =>
      api.handle({
        method: 'PUT',
        path: `/api/editorials/${e.id}`,
        body: { title: 'no', contentMd: 'no' },
      }),
    ).toThrow('操作权限')
    expect(() =>
      api.handle({ method: 'PUT', path: '/api/discussions/1', body: { contentMd: 'no' } }),
    ).toThrow('操作权限')
    api.handle({ method: 'DELETE', path: '/api/discussions/1' })
    expect(api.state.discussions.some((p) => p.id === 1 || p.parentId === 1)).toBe(false)
  })
  it('does not let authors read or modify through an inaccessible parent', () => {
    const { api } = setup(),
      e = api.state.editorials.find((e) => e.authorId === demoUser.id)!
    const post = api.handle({
      method: 'POST',
      path: `/api/editorials/${e.id}/discussions`,
      body: { contentMd: 'my comment' },
    }) as DtoDiscussionResponse
    api.state.problems.find((p) => p.id === e.problemId)!.visibility = 'private'
    expect(() => api.handle({ method: 'GET', path: `/api/editorials/${e.id}` })).toThrow('不存在')
    expect(() =>
      api.handle({
        method: 'PUT',
        path: `/api/discussions/${post.id}`,
        body: { contentMd: 'stale' },
      }),
    ).toThrow('不存在')
    const list = api.handle({ method: 'GET', path: '/api/editorials' }) as {
      items: DtoEditorialResponse[]
    }
    expect(list.items.some((item) => item.id === e.id)).toBe(false)
    expect(JSON.stringify(list)).not.toContain('contentMd')
  })
  it('gates editorial threads, then accepts only same-thread replies after simulated practice AC', () => {
    const { api, advance } = setup(),
      e = api.state.editorials[0]
    e.solvedOnly = true
    api.state.submissions = []
    const path = `/api/editorials/${e.id}/discussions`
    expect(() => api.handle({ method: 'GET', path })).toThrow('通过题目')
    expect(() =>
      api.handle({ method: 'POST', path: `/api/editorials/${e.id}/vote`, body: { up: true } }),
    ).toThrow('操作权限')
    api.handle({
      method: 'POST',
      path: '/api/submissions',
      body: { problemId: e.problemId, language: 'cpp', sourceCode: 'int main() {}' },
    })
    advance()
    const thread = api.handle({ method: 'GET', path }) as {
      items: DtoDiscussionResponse[]
      canPost: boolean
    }
    expect(thread.canPost).toBe(true)
    const root = thread.items[0]
    const reply = api.handle({
      method: 'POST',
      path,
      body: { contentMd: 'reply', parentId: root.id },
    }) as DtoDiscussionResponse
    expect(reply.parentId).toBe(root.id)
    expect(() =>
      api.handle({
        method: 'POST',
        path: `/api/editorials/${api.state.editorials[1].id}/discussions`,
        body: { contentMd: 'wrong', parentId: root.id },
      }),
    ).toThrow('不属于')
    api.handle({ method: 'DELETE', path: `/api/discussions/${root.id}` })
    expect(api.state.discussions.some((p) => p.id === reply.id)).toBe(false)
    const next = api.handle({
      method: 'POST',
      path,
      body: { contentMd: 'new root' },
    }) as DtoDiscussionResponse
    expect(next.id).toBeGreaterThan(reply.id)
  })
  it('stores votes independently for each account and has no contest discussion route', () => {
    const { api } = setup(),
      e = api.state.editorials[0],
      path = `/api/editorials/${e.id}`
    const before = e.voteCount
    api.handle({ method: 'POST', path: `${path}/vote`, body: { up: true } })
    api.state.user = { ...juryUser }
    expect(api.handle({ method: 'GET', path })).toHaveProperty('voted', false)
    api.handle({ method: 'POST', path: `${path}/vote`, body: { up: true } })
    expect(e.voteCount).toBe(before + 2)
    api.state.user = { ...demoUser }
    api.handle({ method: 'POST', path: `${path}/vote`, body: { up: false } })
    api.state.user = { ...juryUser }
    expect(api.handle({ method: 'GET', path })).toHaveProperty('voted', true)
    expect(e.voteCount).toBe(before + 1)
    expect(() =>
      api.handle({ method: 'GET', path: `/api/contests/${api.state.contests[0].id}/discussions` }),
    ).toThrow('澄清')
  })
})
