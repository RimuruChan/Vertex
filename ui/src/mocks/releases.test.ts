import { describe, expect, it } from 'vitest'
import { authoringFixture } from './authoring-fixture'
import { adminUser, demoUser } from './identities'
import type {
  DomainCheckRun,
  DomainCommitOutcome,
  DomainWorkingCopy,
  DtoCopyResponse,
  DtoProblemResponse,
  DtoSubmissionResponse,
} from './models'

describe('explicit publication in mock mode', () => {
  it('publishes wording using a matching check without upgrading queued submissions or contests', () => {
    const f = authoringFixture(),
      { api, problem, path } = f
    const before = problem.statementMd
    const queued = api.handle({
      method: 'POST',
      path: '/api/domains/official/submissions',
      body: { problemId: f.number, language: 'cpp', sourceCode: 'int main(){}' },
    }) as DtoSubmissionResponse
    const checked = f.check()
    f.save('statement-zh', 'New wording')
    expect(problem.statementMd).toBe(before)
    expect(f.copy().headRevision).toBeUndefined()
    const revision = f.commit().revision
    expect(f.publish(revision, checked.id)).toHaveProperty('version', 2)
    expect(f.publish(revision, checked.id)).toHaveProperty('version', 2)
    expect(problem.statementMd).toContain('New wording')
    expect(queued.problemVersion).toBe(1)
    const contestPath = '/api/domains/official/contests/1/problems/1000'
    expect(api.handle({ method: 'GET', path: contestPath })).toMatchObject({
      version: 1,
      statementMd: before,
    })
    api.handle({
      method: 'PUT',
      path: contestPath + '/version',
      body: { version: 2, expectedVersion: 1 },
    })
    expect(api.handle({ method: 'GET', path: contestPath })).toMatchObject({
      version: 2,
      statementMd: problem.statementMd,
    })
    expect(api.handle({ method: 'GET', path: path + '/releases' })).toMatchObject({ total: 1 })
  })
  it('checks sealed inputs and refuses to publish changed data with an older check', () => {
    const f = authoringFixture()
    f.save('input', 'reviewed input')
    const started = f.api.handle({
      method: 'POST',
      path: f.path + '/checks',
      body: { etag: f.copy().etag },
    }) as DomainCheckRun
    f.save('input', 'later input')
    for (let i = 0; i < 3; i++) f.api.handle({ method: 'GET', path: f.path + '/checks' })
    const completed = f.api.handle({
      method: 'GET',
      path: f.path + '/checks/' + started.id,
    }) as DomainCheckRun
    expect(completed.tests?.[0].inputHead).toBe('reviewed input')
    expect(completed.log).toContain('未执行任何程序')
    expect(() => f.publish(f.commit().revision, completed.id)).toThrow('匹配')
    expect(f.problem.publishedVersion).toBe(1)
  })
  it('retires old write routes and protects referenced problems from deletion', () => {
    const { api, problem } = authoringFixture()
    for (const [method, suffix] of [
      ['GET', '/package'],
      ['PUT', '/statements/zh'],
      ['POST', '/testdata'],
      ['POST', '/builds'],
      ['POST', '/publish'],
      ['PUT', ''],
    ]) {
      expect(() =>
        api.handle({
          method,
          path: '/api/domains/official/admin/problems/1000' + suffix,
          body: { file: new Blob(['invalid']) },
        }),
      ).toThrow('不存在')
    }
    expect(() =>
      api.handle({ method: 'DELETE', path: '/api/domains/official/admin/problems/1000' }),
    ).toThrow('引用')
    expect(api.state.problems).toContain(problem)
  })
  it('publishes reviewed samples without later private edits and preserves live counters', () => {
    const f = authoringFixture()
    f.save('input', 'reviewed input')
    const check = f.check(),
      revision = f.commit().revision
    f.save('input', 'unreviewed input')
    const count = f.problem.submissionCount + 7
    f.problem.submissionCount = count
    expect(() =>
      f.api.handle({
        method: 'POST',
        path: f.path + '/releases',
        body: { revision, checkId: check.id, expectedVersion: 1, language: 'ja' },
      }),
    ).toThrow('语言')
    f.publish(revision, check.id)
    expect(f.problem.submissionCount).toBe(count)
    expect(f.problem.statementMd).toContain('reviewed input')
    expect(f.problem.statementMd).not.toContain('unreviewed input')
  })
  it('keeps unpublished wording in the author workspace and rejects public reads and submissions', () => {
    const { api } = authoringFixture()
    const item = api.handle({
      method: 'POST',
      path: '/api/domains/official/admin/problems',
      body: { title: 'Unreleased', visibility: 'public' },
    }) as DtoProblemResponse
    const f = authoringFixture(api, item.id)
    f.save('statement-zh', 'Private unreleased body')
    const entry = f.copy().tree.entries.find((e) => e.id === 'statement-zh')!
    expect(api.handle({ method: 'GET', path: f.path + '/blobs/' + entry.blob.sha256 })).toEqual({
      mockBlob: [...new TextEncoder().encode('Private unreleased body')],
    })
    expect(() =>
      api.handle({
        method: 'POST',
        path: '/api/domains/official/submissions',
        body: { problemId: item.id, language: 'cpp', sourceCode: 'int main(){}' },
      }),
    ).toThrow('尚未发布')
    api.state.user = null
    expect(() =>
      api.handle({ method: 'GET', path: '/api/domains/official/problems/' + item.id }),
    ).toThrow()
    expect(() =>
      api.handle({ method: 'GET', path: f.path + '/blobs/' + entry.blob.sha256 }),
    ).toThrow()
    expect(
      api.handle({
        method: 'GET',
        path: '/api/domains/official/problems',
        params: { keyword: 'Unreleased' },
      }),
    ).toMatchObject({ items: [] })
  })
  it('copies reviewed material independently and can publish it after the source is deleted', () => {
    const { api } = authoringFixture()
    const item = api.handle({
      method: 'POST',
      path: '/api/domains/official/admin/problems',
      body: { title: 'Copy source' },
    }) as DtoProblemResponse
    const f = authoringFixture(api, item.id)
    f.save('source', 'reviewed code')
    const check = f.check()
    f.publish(f.commit().revision, check.id)
    f.save('source', 'later private code')
    const copied = api.handle({
      method: 'POST',
      path: '/api/domains/training/authoring/problem-copies',
      body: {
        sourceDomain: 'official',
        sourceProblem: item.id,
        sourceVersion: 1,
        attribution: 'Approved for training',
      },
    }) as DtoCopyResponse
    const target = `/api/domains/training/authoring/problems/${copied.problemId}`
    const copy = api.handle({ method: 'GET', path: target + '/working-copy' }) as DomainWorkingCopy
    const entry = copy.tree.entries.find((e) => e.id === 'source')!
    expect(api.handle({ method: 'GET', path: target + '/blobs/' + entry.blob.sha256 })).toEqual({
      mockBlob: [...new TextEncoder().encode('reviewed code')],
    })
    api.handle({ method: 'DELETE', path: '/api/domains/official/admin/problems/' + item.id })
    expect(api.handle({ method: 'GET', path: target + '/origin' })).toMatchObject({
      origin: {
        sourceProblemNumber: item.id,
        sourceVersion: 1,
        attribution: 'Approved for training',
      },
    })
    const commit = api.handle({
      method: 'POST',
      path: target + '/commits',
      body: { etag: copy.etag, message: 'Adopt source', requestId: 'adopt' },
    }) as DomainCommitOutcome
    const checks = api.handle({ method: 'GET', path: target + '/checks' }) as {
      items: DomainCheckRun[]
    }
    expect(
      api.handle({
        method: 'POST',
        path: target + '/releases',
        body: {
          revision: commit.commit!.revision,
          checkId: checks.items[0].id,
          expectedVersion: 0,
        },
      }),
    ).toHaveProperty('version', 1)
  })
  it('requires source package access and target creation rights independently', () => {
    const f = authoringFixture(),
      { api } = f
    const check = f.check(),
      published = f.publish(f.commit().revision, check.id)
    const body = {
      sourceDomain: 'official',
      sourceProblem: f.number,
      sourceVersion: published.version,
      attribution: 'Training copy',
    }
    api.state.user = { ...demoUser }
    expect(() =>
      api.handle({ method: 'POST', path: '/api/domains/training/authoring/problem-copies', body }),
    ).toThrow('源题目包')
    api.state.user = { ...adminUser }
    api.handle({
      method: 'PUT',
      path: '/api/domains/official/admin/problems/1000/access',
      body: { username: 'demo', role: 'reader' },
    })
    api.state.user = { ...demoUser }
    expect(() =>
      api.handle({ method: 'POST', path: '/api/domains/official/authoring/problem-copies', body }),
    ).toThrow('目标域')
    expect(
      api.handle({ method: 'POST', path: '/api/domains/training/authoring/problem-copies', body }),
    ).toHaveProperty('domainSlug', 'training')
  })
})
