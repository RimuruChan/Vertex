import { describe, expect, it } from 'vitest'
import type { DtoProblemResponse, DomainWorkingCopy } from './models'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser } from './identities'
import { authoringFixture } from './authoring-fixture'

const now = Date.UTC(2026, 8, 7)

describe('mock authoring permissions', () => {
  it('checks resource access and resolves public numbers without exposing private identifiers', () => {
    const api = createMockAPI(createFixtures(now), () => now)
    const path = '/api/domains/official/authoring/problems/1000'
    expect(() => api.handle({ method: 'POST', path: path + '/working-copy' })).toThrow('协作权限')
    api.state.user = { ...adminUser }
    const copy = api.handle({ method: 'POST', path: path + '/working-copy' }) as DomainWorkingCopy
    expect(copy.tree.entries.map((entry) => entry.kind).sort()).toEqual(['metadata', 'statement'])
    expect(() =>
      api.handle({
        method: 'GET',
        path: `/api/domains/official/authoring/problems/${api.state.problems[0].domainId}/working-copy`,
      }),
    ).toThrow()
    expect(copy.headRevision).toBeUndefined()
  })
  it('saves languages independently and keeps private explanations out of publication', () => {
    const f = authoringFixture()
    f.save('statement-en', 'Find two indices.', {
      id: 'statement-en',
      kind: 'statement',
      path: 'statement/problem.en.md',
      attributes: { language: 'en', format: 'markdown' },
    })
    f.save('explanation', 'private explanation', {
      id: 'explanation',
      kind: 'resource',
      path: 'notes/tutorial.md',
      attributes: {},
    })
    expect(f.copy().tree.entries.filter((entry) => entry.kind === 'statement')).toHaveLength(2)
    const checked = f.check()
    f.api.handle({
      method: 'POST',
      path: f.path + '/releases',
      body: {
        revision: f.commit().revision,
        checkId: checked.id,
        expectedVersion: 1,
        language: 'en',
      },
    })
    expect(f.problem.statementMd).toContain('Find two indices.')
    expect(f.problem.statementMd).not.toContain('private explanation')
    f.api.state.user = null
    const raw = f.api.handle({ method: 'GET', path: '/api/domains/official/problems/1000' })
    expect(JSON.stringify(raw)).not.toContain('private explanation')
  })
  it('does not reuse deleted public numbers or silently change existing contest problems', () => {
    const api = createMockAPI(createFixtures(now), () => now)
    api.state.user = { ...adminUser }
    const initial = api.handle({ method: 'GET', path: '/api/domains/official/contests/1' })
    const created = api.handle({
      method: 'POST',
      path: '/api/domains/official/admin/problems',
      body: { title: 'New task' },
    }) as DtoProblemResponse
    api.handle({ method: 'DELETE', path: `/api/domains/official/admin/problems/${created.id}` })
    const next = api.handle({
      method: 'POST',
      path: '/api/domains/official/admin/problems',
      body: { title: 'Next task' },
    }) as DtoProblemResponse
    expect(next.id).not.toBe(created.id)
    expect(api.handle({ method: 'GET', path: '/api/domains/official/contests/1' })).toEqual(initial)
  })
})
