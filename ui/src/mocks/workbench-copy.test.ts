import { describe, expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser } from './identities'
import { createAuthoringExample } from './authoring-example'
import type {
  DomainCheckRun,
  DomainCommitOutcome,
  DomainMaterialView,
  DomainWorkingCopy,
  DtoCopyResponse,
} from '@/generated/api/model'

describe('committed release copy mock', () => {
  it('copies a published snapshot into a private draft with independent blobs and a reusable check', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...adminUser }
    const number = createAuthoringExample(api, 'official'),
      source = `/api/domains/official/authoring/problems/${number}`
    const check = api.handle({
      method: 'POST',
      path: source + '/checks',
      body: { revision: 1 },
    }) as DomainCheckRun
    for (let index = 0; index < 3; index++) api.handle({ method: 'GET', path: source + '/checks' })
    api.handle({
      method: 'POST',
      path: source + '/releases',
      body: { revision: 1, checkId: check.id, expectedVersion: 0 },
    })
    const copy = api.handle({ method: 'GET', path: source + '/working-copy' }) as DomainWorkingCopy
    api.handle({
      method: 'PUT',
      path: source + '/working-copy/entries/private-note',
      body: {
        etag: copy.etag,
        entry: { id: 'private-note', kind: 'source', path: 'notes/private.cpp', attributes: {} },
        text: '// private unpublished note',
      },
    })
    api.handle({
      method: 'POST',
      path: '/api/domains',
      body: {
        slug: 'copy-target',
        name: 'Copy target',
        visibility: 'private',
        joinPolicy: 'invite',
      },
    })
    const result = api.handle({
      method: 'POST',
      path: '/api/domains/copy-target/authoring/problem-copies',
      body: {
        sourceDomain: 'official',
        sourceProblem: number,
        sourceVersion: 1,
        attribution: 'Test release copy',
      },
    }) as DtoCopyResponse
    const destination = `/api/domains/copy-target/authoring/problems/${result.problemId}`
    const copied = api.handle({
      method: 'GET',
      path: destination + '/working-copy',
    }) as DomainWorkingCopy
    expect(copied.headRevision).toBeUndefined()
    expect(copied.tree.entries.some((item) => item.id === 'private-note')).toBe(false)
    expect(
      (
        api.handle({
          method: 'GET',
          path: destination + '/materials/problem',
        }) as DomainMaterialView
      ).metadata?.title,
    ).toBe('出题工作台 · A + B')
    const checks = api.handle({ method: 'GET', path: destination + '/checks' }) as {
      items: DomainCheckRun[]
    }
    expect(checks.items).toHaveLength(1)
    expect(checks.items[0]).toMatchObject({ state: 'succeeded', stage: 'copied' })
    const adopted = api.handle({
      method: 'POST',
      path: destination + '/commits',
      body: { etag: copied.etag, message: 'Adopt source release', requestId: 'adopt' },
    }) as DomainCommitOutcome
    const published = api.handle({
      method: 'POST',
      path: destination + '/releases',
      body: { revision: adopted.commit!.revision, checkId: checks.items[0].id, expectedVersion: 0 },
    })
    expect(published).toMatchObject({ version: 1, revision: 1 })
    expect(api.handle({ method: 'GET', path: destination + '/origin' })).toMatchObject({
      origin: { sourceProblemNumber: number, sourceVersion: 1, attribution: 'Test release copy' },
    })
  })
})
