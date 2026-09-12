import { describe, expect, it } from 'vitest'
import { contestSections, contestSectionPath } from './contest-routes'

describe('contest section paths', () => {
  it.each(Object.entries(contestSections))('round trips %s', (section, tab) => {
    expect(contestSectionPath('42', tab)).toBe(`/contests/42/${section}`)
  })
  it('uses the same standings path for jury and participant views', () => {
    expect(contestSectionPath('42', 'board')).toBe(contestSectionPath('42', 'rankboard'))
  })
  it('keeps rejudging independent of submissions', () => {
    expect(contestSectionPath('42', 'rejudge')).toBe('/contests/42/rejudge')
  })
})
