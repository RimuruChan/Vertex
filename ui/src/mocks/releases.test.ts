import { describe, expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser } from './identities'
import { demoUser } from './identities'
import type {
  DtoWorkspaceResponse,
  DtoProblemResponse,
  DtoSubmissionResponse,
} from '@/generated/api/model'

describe('explicit publication in mock mode', () => {
  const setup = () => {
    let now = Date.UTC(2026, 8, 7, 12)
    const api = createMockAPI(createFixtures(now), () => now)
    api.state.user = { ...adminUser }
    const problem = api.state.problems[0],
      path = `/api/admin/problems/${problem.id}`
    const workspace = () =>
      api.handle({ method: 'GET', path: `${path}/package` }) as DtoWorkspaceResponse
    return {
      api,
      problem,
      path,
      workspace,
      advance: () => {
        now += 6000
      },
    }
  }
  it('publishes saved wording explicitly without rebuilding matching data or upgrading a contest', () => {
    const { api, problem, path, workspace } = setup(),
      before = problem.statementMd
    const queued = api.handle({
      method: 'POST',
      path: '/api/submissions',
      body: { problemId: problem.id, language: 'cpp', sourceCode: 'int main(){}' },
    }) as DtoSubmissionResponse
    workspace()
    api.handle({
      method: 'PUT',
      path: `${path}/statements/zh`,
      body: { name: 'Saved draft', legend: 'New wording' },
    })
    const current = workspace()
    expect(problem.statementMd).toBe(before)
    expect(current.meta.stale).toBe(false)
    expect(current.meta.unpublishedChanges).toBe(true)
    const input = {
      revision: current.meta.packageRevision,
      artifactVersion: current.meta.testdataVersion,
    }
    expect(api.handle({ method: 'POST', path: `${path}/publish`, body: input })).toHaveProperty(
      'version',
      2,
    )
    expect(api.handle({ method: 'POST', path: `${path}/publish`, body: input })).toHaveProperty(
      'version',
      2,
    )
    expect(problem.statementMd).toContain('New wording')
    expect(queued.problemVersion).toBe(1)
    const contest = api.state.contests[0]
    const pinned = api.handle({
      method: 'GET',
      path: `/api/contests/${contest.id}/problems/${problem.id}`,
    }) as DtoProblemResponse & { version: number }
    expect(pinned.version).toBe(1)
    expect(pinned.statementMd).toBe(before)
    api.handle({
      method: 'PUT',
      path: `/api/contests/${contest.id}/problems/${problem.id}/version`,
      body: { version: 2, expectedVersion: 1 },
    })
    expect(
      api.handle({ method: 'GET', path: `/api/contests/${contest.id}/problems/${problem.id}` }),
    ).toHaveProperty('version', 2)
  })
  it('builds a sealed input and does not replace candidates with a stale data revision', () => {
    const { api, path, workspace, advance } = setup()
    const initial = workspace()
    api.handle({
      method: 'PUT',
      path: `${path}/tests/1`,
      body: { source: 'manual', inputData: 'old input', isSample: true },
    })
    api.handle({ method: 'POST', path: `${path}/builds` })
    api.handle({
      method: 'PUT',
      path: `${path}/tests/1`,
      body: { source: 'manual', inputData: 'new input', isSample: true },
    })
    advance()
    const built = workspace()
    expect(built.latestBuild?.tests[0].inputHead).toBe('old input')
    expect(built.meta.testdataVersion).toBe(initial.meta.testdataVersion)
    expect(built.meta.stale).toBe(true)
    expect(() =>
      api.handle({
        method: 'POST',
        path: `${path}/publish`,
        body: { revision: built.meta.packageRevision, artifactVersion: built.meta.testdataVersion },
      }),
    ).toThrow('已变化')
  })
  it('treats imported ZIP files as candidates and protects referenced problems from deletion', () => {
    const { api, problem, path, workspace } = setup()
    const before = workspace().meta.publishedVersion
    const archive = new Blob(['mock fixture'], { type: 'application/zip' })
    api.handle({
      method: 'POST',
      path: `${path}/testdata`,
      body: { file: archive, checker: 'diff' },
    })
    expect(workspace().meta.publishedVersion).toBe(before)
    expect(workspace().meta.unpublishedChanges).toBe(true)
    expect(() => api.handle({ method: 'DELETE', path })).toThrow('引用')
    expect(api.state.problems.some((p) => p.id === problem.id)).toBe(true)
  })
  it('renders candidate samples without replacing them with edited tests or resetting live counters', () => {
    const { api, problem, path, workspace, advance } = setup()
    workspace()
    api.handle({
      method: 'PUT',
      path: `${path}/tests/1`,
      body: { source: 'manual', inputData: 'reviewed input', isSample: true },
    })
    api.handle({ method: 'POST', path: `${path}/builds` })
    advance()
    workspace()
    api.handle({
      method: 'PUT',
      path: `${path}/tests/1`,
      body: { source: 'manual', inputData: 'later input', isSample: true },
    })
    api.handle({ method: 'POST', path: `${path}/builds` })
    api.handle({
      method: 'PUT',
      path: `${path}/tests/1`,
      body: { source: 'manual', inputData: 'even later input', isSample: true },
    })
    advance()
    workspace()
    const preview = api.handle({
      method: 'POST',
      path: `${path}/statements/zh/preview`,
      body: { name: 'Preview', legend: 'Body' },
    }) as { statementMd: string }
    expect(preview.statementMd).toContain('reviewed input')
    expect(preview.statementMd).not.toContain('later input')
    api.handle({ method: 'POST', path: `${path}/builds` })
    advance()
    const meta = workspace().meta
    const count = problem.submissionCount + 7
    problem.submissionCount = count
    const input = { revision: meta.packageRevision, artifactVersion: meta.testdataVersion }
    expect(() =>
      api.handle({ method: 'POST', path: `${path}/publish`, body: { ...input, language: 'ja' } }),
    ).toThrow('所选语言')
    api.handle({ method: 'POST', path: `${path}/publish`, body: input })
    expect(problem.submissionCount).toBe(count)
    expect(problem.statementMd).toContain('even later input')
  })
  it('lets the owner preview an unreleased working copy without admitting public reads or submissions', () => {
    const { api } = setup()
    const item = api.handle({
      method: 'POST',
      path: '/api/admin/problems',
      body: { title: 'New task', visibility: 'public' },
    }) as DtoProblemResponse
    const route = `/api/admin/problems/${item.id}`
    api.handle({
      method: 'PUT',
      path: `${route}/statements/zh`,
      body: { name: 'New draft title', legend: 'Unreleased body' },
    })
    expect(api.handle({ method: 'GET', path: `/api/problems/${item.publicId}` })).toHaveProperty(
      'title',
      'New draft title',
    )
    expect(() =>
      api.handle({
        method: 'POST',
        path: '/api/submissions',
        body: { problemId: item.id, language: 'cpp', sourceCode: 'int main(){}' },
      }),
    ).toThrow('尚未发布')
    api.state.user = null
    expect(() => api.handle({ method: 'GET', path: `/api/problems/${item.publicId}` })).toThrow()
    const list = api.handle({
      method: 'GET',
      path: '/api/problems',
      params: { keyword: 'New' },
    }) as { items: DtoProblemResponse[] }
    expect(list.items).toEqual([])
  })
  it('copies a reviewed package snapshot into an independent draft that survives source deletion', () => {
    const api = createMockAPI(createFixtures())
    api.state.user = { ...adminUser }
    const source = api.handle({
      method: 'POST',
      path: '/api/admin/problems',
      body: { title: 'Copy source' },
    }) as DtoProblemResponse
    const path = `/api/admin/problems/${source.publicId}`
    api.handle({
      method: 'PUT',
      path: path + '/statements/zh',
      body: { name: 'Reviewed title', legend: 'Reviewed wording' },
    })
    api.handle({
      method: 'PUT',
      path: path + '/files',
      body: {
        kind: 'solution',
        name: 'main.cpp',
        language: 'cpp',
        sourceCode: 'reviewed code',
        isActive: true,
      },
    })
    api.handle({
      method: 'POST',
      path: path + '/testdata',
      body: { file: new Blob(['fixture']), checker: 'diff' },
    })
    const meta = (api.handle({ method: 'GET', path: path + '/package' }) as DtoWorkspaceResponse)
      .meta
    api.handle({
      method: 'POST',
      path: path + '/publish',
      body: { revision: meta.packageRevision, artifactVersion: meta.testdataVersion },
    })
    api.handle({
      method: 'PUT',
      path: path + '/files',
      body: {
        kind: 'solution',
        name: 'main.cpp',
        language: 'cpp',
        sourceCode: 'later unpublished code',
        isActive: true,
      },
    })
    const copied = api.handle({
      method: 'POST',
      path: '/api/domains/training/problem-copies',
      body: {
        sourceDomain: 'official',
        sourceProblem: source.publicId,
        sourceVersion: 1,
        attribution: 'Approved for training',
      },
    }) as { problemId: string; problemPublicId: string }
    const target = `/api/domains/training/admin/problems/${copied.problemPublicId}`
    const workspace = api.handle({
      method: 'GET',
      path: target + '/package',
    }) as DtoWorkspaceResponse
    expect(workspace.meta.publishedVersion).toBe(0)
    expect(workspace.meta.title).toBe('Reviewed title')
    expect(workspace.files.find((file) => file.name === 'main.cpp')?.sourceCode).toBe(
      'reviewed code',
    )
    api.handle({ method: 'DELETE', path })
    expect(api.handle({ method: 'GET', path: target + '/origin' })).toMatchObject({
      origin: {
        sourceProblemId: source.id,
        sourceVersion: 1,
        attribution: 'Approved for training',
      },
    })
    expect(
      api.handle({
        method: 'POST',
        path: target + '/publish',
        body: {
          revision: workspace.meta.packageRevision,
          artifactVersion: workspace.meta.testdataVersion,
        },
      }),
    ).toHaveProperty('version', 1)
  })
  it('requires source package access and destination creation rights independently', () => {
    const { api, problem } = setup()
    const body = {
      sourceDomain: 'official',
      sourceProblem: problem.publicId,
      sourceVersion: 1,
      attribution: 'Training copy',
    }
    api.state.user = { ...demoUser }
    expect(() =>
      api.handle({ method: 'POST', path: '/api/domains/training/problem-copies', body }),
    ).toThrow('源题目包')
    api.state.user = { ...adminUser }
    api.handle({
      method: 'PUT',
      path: `/api/admin/problems/${problem.publicId}/access`,
      body: { username: 'demo', role: 'reader' },
    })
    api.state.user = { ...demoUser }
    expect(() =>
      api.handle({ method: 'POST', path: '/api/domains/official/problem-copies', body }),
    ).toThrow('目标域')
    expect(
      api.handle({ method: 'POST', path: '/api/domains/training/problem-copies', body }),
    ).toHaveProperty('domainSlug', 'training')
  })
})
