import { describe, expect, it } from 'vitest'
import { problemDraftKey } from './problem-draft'

describe('problem draft isolation', () => {
  it('separates equal numbers across domains, accounts, languages and mock mode', () => {
    const keys = [
      problemDraftKey('official', 'alice', '1000', 'cpp'),
      problemDraftKey('training', 'alice', '1000', 'cpp'),
      problemDraftKey('official', 'bob', '1000', 'cpp'),
      problemDraftKey('official', undefined, '1000', 'cpp'),
      problemDraftKey('official', 'alice', '1000', 'python'),
      problemDraftKey('official', 'alice', '1000', 'cpp', true),
    ]
    expect(new Set(keys).size).toBe(keys.length)
    expect(problemDraftKey('official', 'alice', '1000', 'cpp')).toBe(keys[0])
  })
})
