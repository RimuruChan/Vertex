import { describe, expect, it } from 'vitest'
import type { DtoProblemResponse, DtoWorkspaceResponse } from '@/generated/api/model'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'

const now = Date.UTC(2026, 8, 7)

describe('authoring demo', () => {
  it('requires the authoring demo role and resolves public numbers without changing UUIDs', () => {
    const api = createMockAPI(createFixtures(now), () => now)
    expect(() => api.handle({ method: 'GET', path: '/api/admin/problems/1000/package' })).toThrow(
      '出题人',
    )
    api.state.user!.role = 'admin'
    const workspace = api.handle({
      method: 'GET',
      path: '/api/admin/problems/1000/package',
    }) as DtoWorkspaceResponse
    expect(workspace.meta.problemId).toBe(api.state.problems[0].id)
    expect(workspace.statements).toHaveLength(1)
    expect(workspace.files).toHaveLength(1)
    expect(workspace.tests).toHaveLength(1)
  })
  it('saves a language independently, keeps internal tutorials out of previews, and simulates a build', () => {
    let clock = now
    const api = createMockAPI(createFixtures(now), () => clock)
    api.state.user!.role = 'admin'
    const path = '/api/admin/problems/1000'
    const draft = {
      name: 'Two Sum',
      legend: 'Find two indices.',
      inputFormat: 'n and target',
      outputFormat: 'indices',
      tutorial: 'private explanation',
    }
    api.handle({ method: 'PUT', path: path + '/statements/en', body: draft })
    const preview = api.handle({
      method: 'POST',
      path: path + '/statements/en/preview',
      body: draft,
    }) as { statementMd: string }
    expect(preview.statementMd).toContain('Find two indices.')
    expect(preview.statementMd).not.toContain('private explanation')
    let workspace = api.handle({ method: 'GET', path: path + '/package' }) as DtoWorkspaceResponse
    expect(workspace.statements.map((s) => s.language)).toEqual(['zh', 'en'])
    api.handle({ method: 'POST', path: path + '/builds' })
    clock += 5000
    workspace = api.handle({ method: 'GET', path: path + '/package' }) as DtoWorkspaceResponse
    expect(workspace.latestBuild?.state).toBe('succeeded')
    expect(workspace.latestBuild?.log).toContain('未编译或执行程序')
    expect(workspace.meta.stale).toBe(false)
  })
  it('does not reuse deleted public numbers or silently change existing contest problems', () => {
    const api = createMockAPI(createFixtures(now), () => now)
    api.state.user!.role = 'admin'
    const initial = api.handle({ method: 'GET', path: '/api/contests/1' })
    const created = api.handle({
      method: 'POST',
      path: '/api/admin/problems',
      body: { title: 'New task' },
    }) as DtoProblemResponse
    api.handle({ method: 'DELETE', path: `/api/admin/problems/${created.publicId}` })
    const next = api.handle({
      method: 'POST',
      path: '/api/admin/problems',
      body: { title: 'Next task' },
    }) as DtoProblemResponse
    expect(next.publicId).not.toBe(created.publicId)
    expect(api.handle({ method: 'GET', path: '/api/contests/1' })).toEqual(initial)
  })
})
