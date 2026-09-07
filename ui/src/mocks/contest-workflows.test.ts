import { describe, expect, it } from 'vitest'
import type { DtoContestResponse, DtoContestDetailsResponse } from '@/generated/api/model'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, observerUser, contestantUser, demoUser } from './identities'

function setup() {
  let now = Date.parse('2030-01-01T00:00:00Z')
  const api = createMockAPI(createFixtures(now), () => now)
  api.state.user = { ...adminUser }
  const input = {
    title: 'Workflow round',
    beginAt: '2030-01-02T00:00:00Z',
    endAt: '2030-01-02T05:00:00Z',
    visibility: 'private',
    rule: 'icpc',
    admission: 'members',
  }
  const event = api.handle({
    method: 'POST',
    path: '/api/admin/contests',
    body: input,
  }) as DtoContestResponse
  const path = `/api/contests/${event.publicId}`,
    admin = `/api/admin/contests/${event.publicId}`
  return {
    api,
    event,
    path,
    admin,
    input,
    start: () => {
      now = Date.parse(input.beginAt)
    },
    end: () => {
      now = Date.parse(input.endAt)
    },
  }
}

describe('contest detail workflows', () => {
  it('defaults registration switches and preserves explicit false and omitted settings', () => {
    const { api, event, admin, input } = setup()
    expect(event).toMatchObject({ allowSelfRegistration: true, allowLateRegistration: false })
    api.handle({
      method: 'PUT',
      path: admin,
      body: { ...input, allowSelfRegistration: false, allowLateRegistration: true },
    })
    const updated = api.handle({ method: 'PUT', path: admin, body: input }) as DtoContestResponse
    expect(updated).toMatchObject({ allowSelfRegistration: false, allowLateRegistration: true })
    expect(() =>
      api.handle({ method: 'PUT', path: admin, body: { ...input, allowLateRegistration: 'yes' } }),
    ).toThrow('布尔值')
  })
  it('opens late registration only when configured and keeps already registered users after closure', () => {
    const { api, path, admin, input, event, start, end } = setup()
    api.handle({ method: 'PUT', path: admin, body: { ...input, visibility: 'public' } })
    start()
    api.state.user = { ...contestantUser }
    expect(() => api.handle({ method: 'POST', path: path + '/register' })).toThrow('报名已经结束')
    api.state.user = { ...adminUser }
    api.handle({
      method: 'PUT',
      path: admin,
      body: { ...input, visibility: 'public', allowLateRegistration: true },
    })
    api.state.user = { ...contestantUser }
    expect(api.handle({ method: 'GET', path })).toMatchObject({
      contest: { permissions: { register: true } },
    })
    expect(api.handle({ method: 'POST', path: path + '/register' })).toHaveProperty('status', 'ok')
    api.state.user = { ...adminUser }
    api.handle({
      method: 'PUT',
      path: admin,
      body: { ...input, visibility: 'public', allowSelfRegistration: false },
    })
    api.state.user = { ...demoUser }
    expect(() => api.handle({ method: 'POST', path: path + '/register' })).toThrow('自助报名已关闭')
    api.state.user = { ...contestantUser }
    expect(api.handle({ method: 'POST', path: path + '/register' })).toHaveProperty('status', 'ok')
    expect(api.state.registrations[contestantUser.id].filter((id) => id === event.id)).toHaveLength(
      1,
    )
    expect(api.handle({ method: 'GET', path })).toMatchObject({
      contest: { permissions: { submit: true, register: false } },
    })
    api.state.user = { ...adminUser }
    api.handle({
      method: 'PUT',
      path: admin,
      body: {
        ...input,
        visibility: 'public',
        allowSelfRegistration: true,
        allowLateRegistration: true,
      },
    })
    end()
    api.state.user = { ...demoUser }
    expect(() => api.handle({ method: 'POST', path: path + '/register' })).toThrow('报名已经结束')
  })
  it('does not let editors change registration policy or late entry bypass passwords', () => {
    const { api, path, admin, input, start } = setup()
    const protectedInput = {
      ...input,
      visibility: 'password',
      password: 'round-fixture',
      allowLateRegistration: true,
    }
    api.handle({ method: 'PUT', path: admin, body: protectedInput })
    api.handle({
      method: 'PUT',
      path: path + '/access',
      body: { username: 'observer', role: 'editor' },
    })
    api.state.user = { ...observerUser }
    expect(() =>
      api.handle({
        method: 'PUT',
        path: admin,
        body: { ...protectedInput, password: undefined, allowSelfRegistration: false },
      }),
    ).toThrow('owner')
    expect(() =>
      api.handle({
        method: 'PUT',
        path: admin,
        body: { ...protectedInput, password: undefined, allowLateRegistration: false },
      }),
    ).toThrow('owner')
    start()
    expect(() =>
      api.handle({ method: 'POST', path: path + '/register', body: { password: 'round-fixture' } }),
    ).toThrow('当前身份')
    api.state.user = { ...contestantUser }
    expect(() =>
      api.handle({ method: 'POST', path: path + '/register', body: { password: 'wrong' } }),
    ).toThrow('密码')
    expect(
      api.handle({ method: 'POST', path: path + '/register', body: { password: 'round-fixture' } }),
    ).toHaveProperty('status', 'ok')
  })
  it('searches before pagination and does not mix same-named contests across domains', () => {
    const { api, event, input } = setup()
    api.handle({ method: 'POST', path: '/api/domains/training/admin/contests', body: input })
    expect(
      api.handle({
        method: 'GET',
        path: '/api/admin/contests',
        params: { keyword: 'Workflow', size: 1 },
      }),
    ).toMatchObject({ total: 1, items: [{ id: event.id }] })
    api.state.user = { ...observerUser }
    expect(
      api.handle({ method: 'GET', path: '/api/admin/contests', params: { keyword: 'Workflow' } }),
    ).toMatchObject({ total: 0, items: [] })
  })
  it('preserves labels, points, colors and adopted versions when saving an explicit composition', () => {
    const { api, admin, path } = setup(),
      [a, b] = api.state.problems
    const entries = [
      { problemId: a.id, label: 'Warmup', points: 30, color: 'red' },
      { problemId: b.id, label: 'B2', points: 250, color: '#123456' },
    ]
    api.handle({ method: 'PUT', path: admin + '/problems', body: { problems: entries } })
    a.publishedVersion = 2
    api.handle({
      method: 'PUT',
      path: admin + '/problems',
      body: { problems: [...entries].reverse() },
    })
    const details = api.handle({ method: 'GET', path }) as DtoContestDetailsResponse
    expect(
      details.problems.map(({ label, points, color, version }) => ({
        label,
        points,
        color,
        version,
      })),
    ).toEqual([
      { label: 'B2', points: 250, color: '#123456', version: 1 },
      { label: 'Warmup', points: 30, color: 'red', version: 1 },
    ])
    expect(() =>
      api.handle({
        method: 'PUT',
        path: admin + '/problems',
        body: { problems: [{ ...entries[0], label: '1000' }] },
      }),
    ).toThrow('题目编号')
  })
  it('allows editor preparation without password changes and denies ownership-only or live mutations', () => {
    const { api, input, admin, path, start } = setup()
    api.handle({
      method: 'PUT',
      path: admin,
      body: { ...input, visibility: 'password', password: 'fixture' },
    })
    api.handle({
      method: 'PUT',
      path: path + '/access',
      body: { username: 'observer', role: 'editor' },
    })
    api.state.user = { ...observerUser }
    const update = { ...input, visibility: 'password', title: 'Prepared' }
    expect(api.handle({ method: 'PUT', path: admin, body: update })).toHaveProperty(
      'title',
      'Prepared',
    )
    for (const denied of [
      { ...update, password: 'changed' },
      { ...update, visibility: 'public' },
      { ...update, admission: 'restricted' },
    ])
      expect(() => api.handle({ method: 'PUT', path: admin, body: denied })).toThrow('owner')
    start()
    expect(() => api.handle({ method: 'PUT', path: admin, body: update })).toThrow('开赛前')
    expect(() =>
      api.handle({ method: 'PUT', path: admin + '/problems', body: { problems: [] } }),
    ).toThrow('开赛前')
  })
  it('keeps problem metadata hidden from unregistered and pre-start contestants', () => {
    const { api, input, path, admin, start } = setup()
    api.handle({ method: 'PUT', path: admin, body: { ...input, visibility: 'public' } })
    api.handle({
      method: 'PUT',
      path: admin + '/problems',
      body: { problems: [{ problemId: api.state.problems[0].id, label: 'A', points: 100 }] },
    })
    api.state.user = { ...contestantUser }
    expect(api.handle({ method: 'GET', path })).toHaveProperty('problems', [])
    api.handle({ method: 'POST', path: path + '/register' })
    expect(api.handle({ method: 'GET', path })).toHaveProperty('problems', [])
    start()
    expect(
      (api.handle({ method: 'GET', path }) as DtoContestDetailsResponse).problems,
    ).toHaveLength(1)
  })
  it('rejects history deletion but allows the owner to remove an unused contest', () => {
    const { api, input, path, admin } = setup()
    const unused = api.handle({
      method: 'POST',
      path: '/api/admin/contests',
      body: { ...input, title: 'Unused' },
    }) as DtoContestResponse
    api.handle({ method: 'DELETE', path: `/api/contests/${unused.publicId}` })
    expect(() => api.handle({ method: 'GET', path: `/api/contests/${unused.publicId}` })).toThrow()
    api.handle({ method: 'PUT', path: admin, body: { ...input, visibility: 'public' } })
    api.state.user = { ...contestantUser }
    api.handle({ method: 'POST', path: path + '/register' })
    api.state.user = { ...adminUser }
    expect(() => api.handle({ method: 'DELETE', path })).toThrow('历史')
    api.state.user = { ...contestantUser }
    expect(api.handle({ method: 'GET', path: path + '/registration' })).toHaveProperty(
      'registered',
      true,
    )
  })
})
