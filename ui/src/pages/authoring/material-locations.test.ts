import { expect, it } from 'vitest'
import type { DomainTreeEntry } from '@/generated/api/model'
import { availableLocation, programDirectory } from './material-locations'

it('allocates around imported names and files shadowing a managed directory', () => {
  const entries = [{ path: 'statement/problem.zh.md' }, { path: 'vertex/programs' }]
  expect(availableLocation('statement/problem.zh.md', entries, 'new')).toBe(
    'statement/new-problem.zh.md',
  )
  expect(availableLocation('vertex/programs/p.json', entries, 'new')).toBe('material-new/p.json')
  expect(availableLocation('a.md', [{ path: 'A.md' }], 'new')).toBe('new-a.md')
})

it('keeps imported program layouts and derives a common root for new selections', () => {
  const entries = [
    { id: 'main', path: 'program/main.cpp' },
    { id: 'header', path: 'program/include/a.h' },
    { id: 'other', path: 'other/extra.cpp' },
  ] as DomainTreeEntry[]
  expect(programDirectory(['main', 'header'], entries)).toBe('program')
  expect(programDirectory(['header'], entries, 'program')).toBe('program')
  expect(programDirectory(['main', 'other'], entries, 'program')).toBe('')
})
