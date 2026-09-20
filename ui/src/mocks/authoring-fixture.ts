import { createMockAPI } from './api'
import { createFixtures } from './fixtures'
import { adminUser } from './identities'
import { defaultProgram, defaultTest } from '@/lib/authoring-materials'
import type {
  DomainCheckRun,
  DomainCommitOutcome,
  DomainCommitRelease,
  DomainMaterialView,
  DomainTreeEntry,
  DomainWorkingCopy,
} from '@/generated/api/model'

// API-level test fixture. Mock checks only simulate state transitions; they do not execute code.
export function authoringFixture(api = createMockAPI(createFixtures()), number = '1000') {
  api.state.user = { ...adminUser }
  const path = `/api/domains/official/authoring/problems/${number}`
  const problem = api.state.problems.find((item) => item.publicId === number || item.id === number)!
  const copy = () =>
    api.handle({ method: 'GET', path: path + '/working-copy' }) as DomainWorkingCopy
  api.handle({ method: 'POST', path: path + '/working-copy' })
  function save(id: string, text: string, definition?: Omit<DomainTreeEntry, 'blob'>) {
    const current = copy()
    const entry = definition ?? current.tree.entries.find((item) => item.id === id)!
    return api.handle({
      method: 'PUT',
      path: path + '/working-copy/entries/' + id,
      body: { etag: current.etag, entry, text },
    }) as DomainWorkingCopy
  }
  save('source', 'int main(){}', {
    id: 'source',
    kind: 'source',
    path: 'solutions/main.cpp',
    attributes: {},
  })
  save(
    'reference',
    JSON.stringify({
      ...defaultProgram(),
      name: '参考解',
      role: 'solution',
      files: ['source'],
      entryPoint: 'source',
      directory: 'solutions',
      expectedVerdicts: ['Accepted'],
    }),
    {
      id: 'reference',
      kind: 'program',
      path: 'vertex/programs/reference.json',
      attributes: { format: 'json' },
    },
  )
  save('input', '1 2\n', { id: 'input', kind: 'input', path: 'data/1.in', attributes: {} })
  save('answer', '3\n', { id: 'answer', kind: 'answer', path: 'data/1.ans', attributes: {} })
  save(
    'test',
    JSON.stringify({
      ...defaultTest(),
      name: '样例',
      isSample: true,
      input: { kind: 'file', entry: 'input', generator: '', arguments: [] },
      answer: { kind: 'file', entry: 'answer', solution: '' },
    }),
    { id: 'test', kind: 'test', path: 'vertex/tests/test.json', attributes: { format: 'json' } },
  )
  const metadata = (
    api.handle({ method: 'GET', path: path + '/materials/problem' }) as DomainMaterialView
  ).metadata!
  save('problem', JSON.stringify({ ...metadata, mainSolution: 'reference' }))
  const commit = () =>
    (
      api.handle({
        method: 'POST',
        path: path + '/commits',
        body: { etag: copy().etag, message: 'Reviewed fixture', requestId: crypto.randomUUID() },
      }) as DomainCommitOutcome
    ).commit!
  function check() {
    const started = api.handle({
      method: 'POST',
      path: path + '/checks',
      body: { etag: copy().etag },
    }) as DomainCheckRun
    for (let index = 0; index < 3; index++) api.handle({ method: 'GET', path: path + '/checks' })
    return api.handle({ method: 'GET', path: path + '/checks/' + started.id }) as DomainCheckRun
  }
  const publish = (revision: number, checkId: string) =>
    api.handle({
      method: 'POST',
      path: path + '/releases',
      body: { revision, checkId, expectedVersion: problem.publishedVersion },
    }) as DomainCommitRelease
  return { api, path, number, problem, copy, save, commit, check, publish }
}
