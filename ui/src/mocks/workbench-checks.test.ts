import { describe, expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser, demoUser } from './identities'
import { defaultProgram, defaultTest } from '@/lib/authoring-materials'
import type {
  DomainCheckRun,
  DomainCommitOutcome,
  DomainCommitRelease,
  DomainMaterialInspection,
  DomainMaterialView,
  DomainTreeEntry,
  DomainWorkingCopy,
} from '@/generated/api/model'

const path = '/api/domains/official/authoring/problems/1000'
function setup() {
  const api = createMockAPI(createFixtures())
  api.state.user = { ...adminUser }
  let copy = api.handle({ method: 'POST', path: path + '/working-copy' }) as DomainWorkingCopy
  function save(entry: Omit<DomainTreeEntry, 'blob'>, text: string) {
    copy = api.handle({
      method: 'PUT',
      path: path + '/working-copy/entries/' + entry.id,
      body: { etag: copy.etag, entry, text },
    }) as DomainWorkingCopy
    return copy
  }
  const program = {
    ...defaultProgram(),
    name: '参考解',
    role: 'solution',
    expectedVerdicts: ['Accepted'],
    files: ['source'],
    entryPoint: 'source',
    directory: 'solutions',
  }
  save({ id: 'source', kind: 'source', path: 'solutions/main.cpp', attributes: {} }, 'int main(){}')
  save(
    {
      id: 'reference',
      kind: 'program',
      path: 'vertex/programs/reference.json',
      attributes: { format: 'json' },
    },
    JSON.stringify(program),
  )
  save({ id: 'input', kind: 'input', path: 'data/1.in', attributes: {} }, '1 2\n')
  save({ id: 'answer', kind: 'answer', path: 'data/1.ans', attributes: {} }, '3\n')
  save(
    { id: 'test', kind: 'test', path: 'vertex/tests/test.json', attributes: { format: 'json' } },
    JSON.stringify({
      ...defaultTest(),
      name: '样例',
      isSample: true,
      input: { kind: 'file', entry: 'input', generator: '', arguments: [] },
      answer: { kind: 'file', entry: 'answer', solution: '' },
    }),
  )
  const meta = api.handle({
    method: 'GET',
    path: path + '/materials/problem',
  }) as DomainMaterialView
  save({ ...meta.entry }, JSON.stringify({ ...meta.metadata, mainSolution: 'reference' }))
  return {
    api,
    save,
    get copy() {
      return copy
    },
  }
}
describe('authoring checks and publication mock', () => {
  it('keeps validation cases separate from judge cases and reports demo evidence', () => {
    const fixture = setup(),
      { api, save } = fixture
    const entry = {
      id: 'selftest',
      kind: 'validation',
      path: 'vertex/validation/positive.json',
      attributes: { format: 'json' },
    }
    const definition = {
      schemaVersion: 1,
      name: 'positive output',
      mode: 'valid_output',
      input: 'input',
      answer: 'answer',
      output: 'missing',
    }
    save(entry, JSON.stringify(definition))
    const inspect = () =>
      api.handle({ method: 'GET', path: path + '/inspection' }) as DomainMaterialInspection
    expect(inspect().canBuild).toBe(false)
    save(entry, JSON.stringify({ ...definition, output: 'answer' }))
    expect(inspect()).toMatchObject({ canBuild: true, validationCount: 1, testCount: 1 })
    const started = api.handle({
      method: 'POST',
      path: path + '/checks',
      body: { etag: fixture.copy.etag },
    }) as DomainCheckRun
    for (let i = 0; i < 3; i++) api.handle({ method: 'GET', path: path + '/checks' })
    const check = api.handle({
      method: 'GET',
      path: path + '/checks/' + started.id,
    }) as DomainCheckRun
    expect(check.validation).toEqual([
      expect.objectContaining({
        id: 'selftest',
        actual: 'accepted',
        status: 'ok',
        message: '演示结果，未实际运行校验器',
      }),
    ])
    expect(check.tests).toHaveLength(1)
    expect(
      api.handle({ method: 'GET', path: path + '/materials', params: { kind: 'validation' } }),
    ).toMatchObject({ total: 1 })
  })
  it('keeps a frozen check private, rejects stale publication, and releases explicitly', () => {
    const fixture = setup(),
      { api, save } = fixture
    const before = api.state.problems[0].publishedVersion
    api.handle({
      method: 'PUT',
      path: '/api/domains/official/admin/problems/1000/access',
      body: { username: demoUser.username, role: 'editor' },
    })
    const inspection = api.handle({
      method: 'GET',
      path: path + '/inspection',
      params: { etag: fixture.copy.etag },
    }) as DomainMaterialInspection
    expect(inspection.canBuild).toBe(true)
    const started = api.handle({
      method: 'POST',
      path: path + '/checks',
      body: { etag: fixture.copy.etag },
    }) as DomainCheckRun
    for (let i = 0; i < 3; i++) api.handle({ method: 'GET', path: path + '/checks' })
    const check = api.handle({
      method: 'GET',
      path: path + '/checks/' + started.id,
    }) as DomainCheckRun
    expect(check.state).toBe('succeeded')
    expect(check.log).toContain('未执行任何程序')
    expect(api.state.problems[0].publishedVersion).toBe(before)
    api.state.user = { ...demoUser }
    expect(
      (api.handle({ method: 'GET', path: path + '/checks' }) as { items: unknown[] }).items,
    ).toEqual([])
    expect(() => api.handle({ method: 'GET', path: path + '/checks/' + check.id })).toThrow(
      '检查不存在',
    )
    api.state.user = { ...adminUser }
    save({ id: 'answer', kind: 'answer', path: 'data/1.ans', attributes: {} }, '4\n')
    const result = api.handle({
      method: 'POST',
      path: path + '/commits',
      body: { etag: fixture.copy.etag, message: 'changed data', requestId: 'one' },
    }) as DomainCommitOutcome
    expect(() =>
      api.handle({
        method: 'POST',
        path: path + '/releases',
        body: { revision: result.commit!.revision, checkId: check.id, expectedVersion: before },
      }),
    ).toThrow('成功检查')
    // Refresh the token after the explicit commit, then restore the original answer.
    const restored = api.handle({
      method: 'PUT',
      path: path + '/working-copy/entries/answer/text',
      body: { etag: result.copy.etag, text: '3\n' },
    }) as DomainWorkingCopy
    const shared = api.handle({
      method: 'POST',
      path: path + '/commits',
      body: { etag: restored.etag, message: 'checked data', requestId: 'two' },
    }) as DomainCommitOutcome
    api.state.user = { ...demoUser }
    expect(
      (api.handle({ method: 'GET', path: path + '/checks/' + check.id }) as DomainCheckRun)
        .matchingRevision,
    ).toBe(shared.commit!.revision)
    expect(() =>
      api.handle({
        method: 'POST',
        path: path + '/releases',
        body: { revision: shared.commit!.revision, checkId: check.id, expectedVersion: before },
      }),
    ).toThrow('发布权限')
    api.state.user = { ...adminUser }
    const payload = {
      revision: shared.commit!.revision,
      checkId: check.id,
      expectedVersion: before,
      language: 'zh',
    }
    const release = api.handle({
      method: 'POST',
      path: path + '/releases',
      body: payload,
    }) as DomainCommitRelease
    expect(release.version).toBe(before + 1)
    expect(
      (
        api.handle({
          method: 'POST',
          path: path + '/releases',
          body: payload,
        }) as DomainCommitRelease
      ).version,
    ).toBe(release.version)
    expect(
      (api.handle({ method: 'GET', path: path + '/releases' }) as { items: unknown[] }).items,
    ).toHaveLength(1)
  })
  it('keeps cancelled checks terminal and rejects a stale copy token', () => {
    const fixture = setup(),
      { api } = fixture
    expect(() =>
      api.handle({ method: 'POST', path: path + '/checks', body: { etag: 'stale' } }),
    ).toThrow('工作副本已变化')
    const check = api.handle({
      method: 'POST',
      path: path + '/checks',
      body: { etag: fixture.copy.etag },
    }) as DomainCheckRun
    api.handle({ method: 'POST', path: path + '/checks/' + check.id + '/cancel' })
    for (let i = 0; i < 4; i++) api.handle({ method: 'GET', path: path + '/checks' })
    expect(
      (api.handle({ method: 'GET', path: path + '/checks/' + check.id }) as DomainCheckRun).state,
    ).toBe('cancelled')
  })
})
