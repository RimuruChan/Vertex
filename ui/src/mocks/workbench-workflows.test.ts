import { expect, it } from 'vitest'
import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser } from './identities'
import { createAuthoringExample } from './authoring-example'
import { placeMockSamples } from './statement-samples'
import { statementSections } from '@/lib/statement-structure'
import type {
  DomainContentComparison,
  DomainDraftStatementPreview,
  DomainGenerationResult,
  DomainMaterialInspection,
  DomainMaterialView,
  DomainWorkingCopy,
} from '@/generated/api/model'

function fixture(scenario: 'complete' | 'grouped' = 'complete') {
  const api = createMockAPI(createFixtures())
  api.state.user = { ...adminUser }
  const id = createAuthoringExample(api, 'official', scenario),
    endpoint = `/api/domains/official/authoring/problems/${id}`
  const copy = () =>
    api.handle({ method: 'GET', path: endpoint + '/working-copy' }) as DomainWorkingCopy
  return { api, endpoint, copy }
}
it('covers a complete multi-file, multilingual authoring workflow and bounded generation', () => {
  const f = fixture(),
    before = f.copy()
  expect(before.tree.entries.filter((e) => e.kind === 'statement')).toHaveLength(2)
  const reference = f.api.handle({
    method: 'GET',
    path: f.endpoint + '/materials/accepted',
  }) as DomainMaterialView
  expect(reference.program!.files.length).toBeGreaterThan(8)
  const plan = f.api.handle({
    method: 'GET',
    path: f.endpoint + '/materials/coverage-plan',
  }) as DomainMaterialView
  const request = { etag: before.etag, id: 'coverage-plan', plan: plan.generation!, preview: true }
  const preview = f.api.handle({
    method: 'POST',
    path: f.endpoint + '/generation',
    body: request,
  }) as DomainGenerationResult
  expect(preview.cases).toHaveLength(18)
  expect(f.copy()).toEqual(before)
  expect(preview.cases[1].arguments).toEqual(['20', '30', '2', '100'])
  const saved = f.api.handle({
    method: 'POST',
    path: f.endpoint + '/generation',
    body: { ...request, preview: false },
  }) as DomainGenerationResult
  expect(saved.copy!.tree.entries.filter((e) => e.kind === 'test')).toHaveLength(24)
  expect(saved.copy!.tree.entries.find((e) => e.id === 'case-1')).toEqual(
    before.tree.entries.find((e) => e.id === 'case-1'),
  )
  expect(() =>
    f.api.handle({
      method: 'POST',
      path: f.endpoint + '/generation',
      body: {
        ...request,
        etag: f.copy().etag,
        plan: { ...request.plan, rules: [{ ...request.plan.rules[0], count: 501 }] },
      },
    }),
  ).toThrow('规则')
})
it('renders public samples without leaking secret testcase bytes and reviews statement sections', () => {
  const f = fixture(),
    copy = f.copy()
  const preview = f.api.handle({
    method: 'POST',
    path: f.endpoint + '/statement-preview',
    body: {
      etag: copy.etag,
      entryId: 'statement-zh',
      revision: 0,
      content: '# Review\n\n{{remainingsamples}}',
    },
  }) as DomainDraftStatementPreview
  expect(preview.sampleCount).toBe(2)
  expect(preview.content).toContain('1 2 3 4 5')
  expect(JSON.stringify(preview)).not.toContain('1000000000')
  const review = f.api.handle({
    method: 'GET',
    path: f.endpoint + '/changes',
  }) as DomainContentComparison
  const statement = review.review.find((item) => item.kind === 'statement')!
  expect(statement.fields.map((f) => f.key)).toContain('数据范围与提示')
  expect(statement.fields.map((f) => f.key)).not.toContain('题面内容')
})
it('offers a grouped draft with an explicit publication capability warning', () => {
  const f = fixture('grouped'),
    copy = f.copy()
  const inspection = f.api.handle({
    method: 'GET',
    path: f.endpoint + '/inspection',
    params: { etag: copy.etag },
  }) as DomainMaterialInspection
  expect(copy.tree.entries.some((e) => e.kind === 'group')).toBe(true)
  expect(
    inspection.publicationIssues.some((issue) => issue.code === 'publication.unsupported'),
  ).toBe(true)
})
it('keeps sample directives literal inside code and makes title-only edits reviewable', () => {
  const content = '`{{nextsample}}`\n\n```\n{{nextsample}}\n```\n\n{{remainingsamples}}'
  const rendered = placeMockSamples(content, ['SAMPLE {{nextsample}}'])
  expect(rendered).toContain('`{{nextsample}}`')
  expect(rendered).toContain('SAMPLE {{nextsample}}')
  expect(statementSections('# Changed title\n\n## constructor\nbody')).toMatchObject({
    题目标题: 'Changed title',
    constructor: 'body',
  })
})
