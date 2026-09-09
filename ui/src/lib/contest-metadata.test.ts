import { describe, expect, it } from 'vitest'
import { showContestProblemMetadata } from './contest-metadata'

describe('contest problem metadata visibility', () => {
  const contest = { endAt: '2026-09-09T12:00:00Z' }
  const end = Date.parse(contest.endAt)
  it('hides metadata before the end by default, including older cached contests', () => {
    expect(showContestProblemMetadata(contest, end - 1)).toBe(false)
    expect(showContestProblemMetadata({ ...contest, showProblemMetadata: false }, end - 1)).toBe(
      false,
    )
    expect(showContestProblemMetadata(undefined, end)).toBe(false)
  })
  it('honors the opt-in before the end and restores metadata at the end', () => {
    expect(showContestProblemMetadata({ ...contest, showProblemMetadata: true }, end - 1)).toBe(
      true,
    )
    expect(showContestProblemMetadata(contest, end)).toBe(true)
  })
})
